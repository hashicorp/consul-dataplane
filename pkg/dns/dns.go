// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package dns

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/consul/proto-public/pbdns"
	"github.com/hashicorp/go-hclog"
	"golang.org/x/net/dns/dnsmessage"
	"google.golang.org/grpc/metadata"
)

// processStart is a fixed reference point captured at process startup. It's
// used together with time.Since to derive purely monotonic durations for
// tracking listener backoff deadlines, since storing a wall-clock UnixNano
// timestamp would drop Go's monotonic-clock reading and make the backoff
// susceptible to wall-clock corrections/jumps.
var processStart = time.Now()

// ErrServerDisabled is returned when the server is disabled
var ErrServerDisabled error = errors.New("server is disabled")

// ErrServerRunning is returned when the server is already running
var ErrServerRunning error = errors.New("server is already running")

const (
	envoyDNSForwardTimeout     = 300 * time.Millisecond
	listenerUnhealthyTTL       = 5 * time.Second
	inlineListenerUnhealthyTTL = 1 * time.Second
	consulQueryTimeout         = 5 * time.Second
)

// DNSServerParams is the configuration for creating a new DNS server
type DNSServerParams struct {
	BindAddr string
	Port     int
	Logger   hclog.Logger
	Client   pbdns.DNSServiceClient

	Partition  string
	Namespace  string
	Token      string
	Datacenter string
	// VirtualDNSInlineAddr is the address of Envoy's inline DNS listener (127.0.0.1:8653).
	VirtualDNSInlineAddr string
	// VirtualDNSEgressAddr is the address of Envoy's egress DNS listener (127.0.0.1:8654).
	VirtualDNSEgressAddr string
}

// DNSServerInterface is the interface for athe DNSServer
type DNSServerInterface interface {
	Start(context.Context) error
	Stop()
	TcpPort() int
	UdpPort() int
}

// DNSServer is the implementation of the DNSServerInterface
type DNSServer struct {
	bindAddr net.IP
	port     int

	lock    sync.Mutex
	running bool
	cancel  context.CancelFunc

	logger      hclog.Logger
	client      pbdns.DNSServiceClient
	connUDP     net.PacketConn
	listenerTCP net.Listener

	partition            string
	namespace            string
	token                string
	datacenter           string
	virtualDNSInlineAddr string
	virtualDNSEgressAddr string

	// inlineListenerUnavailableTill/egressListenerUnavailableTill store the
	// deadline, expressed as nanoseconds elapsed since processStart (i.e.
	// time.Since(processStart)), until which the corresponding listener
	// should be treated as unavailable (skip forwarding, go straight to
	// Consul). A monotonic elapsed-time value is used instead of a wall-clock
	// UnixNano timestamp so that wall-clock corrections/jumps can't end the
	// backoff early or extend it indefinitely. They're read/written via
	// sync/atomic instead of a mutex since it's a single int64 value accessed
	// concurrently from many query goroutines.
	inlineListenerUnavailableTill atomic.Int64
	egressListenerUnavailableTill atomic.Int64
}

// NewDNSServer creates a new DNS proxy server
func NewDNSServer(p DNSServerParams) (DNSServerInterface, error) {
	if p.Port == -1 {
		return nil, ErrServerDisabled
	}
	s := &DNSServer{}
	s.bindAddr = net.ParseIP(p.BindAddr)
	if s.bindAddr == nil {
		return nil, fmt.Errorf("error parsing specified dns bind addr '%s'", p.BindAddr)
	}
	s.port = p.Port
	s.client = p.Client
	s.logger = p.Logger.Named("dns-proxy")
	s.partition = p.Partition
	s.datacenter = p.Datacenter
	s.virtualDNSInlineAddr = p.VirtualDNSInlineAddr
	s.virtualDNSEgressAddr = p.VirtualDNSEgressAddr
	s.namespace = p.Namespace
	s.token = p.Token
	return s, nil
}

// TcpPort is a helper func for the purpose of returning the port
// that the OS chose if the user specified 0
func (d *DNSServer) TcpPort() int {
	if d.listenerTCP == nil {
		return -1
	}
	return int(d.listenerTCP.Addr().(*net.TCPAddr).Port)
}

// UdpPort is a helper func for the purpose of returning the port
// that the OS chose if the user specified 0 in the server config
func (d *DNSServer) UdpPort() int {
	if d.connUDP == nil {
		return -1
	}
	return int(d.connUDP.LocalAddr().(*net.UDPAddr).Port)
}

// Run starts the tcp and udp listeners and forwards requests to consul
func (d *DNSServer) Start(ctx context.Context) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.logger.Debug("starting DNS proxy", "partition", d.partition, "namespace", d.namespace)

	if d.running {
		return ErrServerRunning
	}

	if d.port == -1 {
		return ErrServerDisabled
	}
	// 1. Setup udp listener
	udpAddr := &net.UDPAddr{
		Port: d.port,
		IP:   d.bindAddr,
	}
	connUDP, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("error listening for udp: %w", err)
	}
	d.connUDP = connUDP

	// 2. Setup tcp listener
	tcpAddr := &net.TCPAddr{
		Port: d.port,
		IP:   d.bindAddr,
	}
	listenerTCP, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return fmt.Errorf("error listening to tcp: %w", err)
	}
	d.listenerTCP = listenerTCP

	runCtx, cancel := context.WithCancel(ctx)
	go d.run(runCtx)

	d.running = true
	d.cancel = cancel

	return nil

}

func (d *DNSServer) run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		d.proxyUDP(ctx)
	}()

	go func() {
		defer wg.Done()
		d.proxyTCP(ctx)
	}()
	d.logger.Info("running dns proxy", " udp port", d.UdpPort(), "tcp port", d.TcpPort())

	wg.Wait()

	d.lock.Lock()
	d.running = false
	d.lock.Unlock()

}

func (d *DNSServer) proxyUDP(ctx context.Context) {
	logger := d.logger.Named("udp")
	for {
		select {
		case <-ctx.Done():
			d.connUDP.Close()
			return
		default:
		}
		buf := make([]byte, 512)
		err := d.connUDP.SetReadDeadline(time.Now().Add(time.Second * 10))
		if err != nil {
			logger.Error("failure to set read deadline on connection", "error", err)
			continue
		}
		bytesRead, addr, err := d.connUDP.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				logger.Info("connection closed")
				return
			} else if nerr, ok := err.(net.Error); !ok || !nerr.Timeout() {
				logger.Warn("error reading from conn", "error", err)
			}
			continue
		}
		// Parallelize responses
		go d.queryConsulAndRespondUDP(buf[0:bytesRead], addr)
	}
}

func (d *DNSServer) queryConsulAndRespondUDP(buf []byte, addr net.Addr) {
	logger := d.logger.Named("udp")

	respMsg, err := d.triageAndResolve(buf, pbdns.Protocol_PROTOCOL_UDP)
	if err != nil {
		logger.Error("error resolving dns request", "error", err)
		return
	}
	_, err = d.connUDP.WriteTo(respMsg, addr)
	if err != nil {
		logger.Error("error sending response", "error", err)
	}
}

func (d *DNSServer) proxyTCP(ctx context.Context) {
	defer d.listenerTCP.Close()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		c, err := d.listenerTCP.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return // listener closed (normal shutdown or context cancel)
			}
			d.logger.Warn("failure to accept tcp connection", "error", err)
			continue // do not dispatch with a nil conn
		}
		go d.proxyTCPAcceptedConn(ctx, c)
	}
}

func (d *DNSServer) proxyTCPAcceptedConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	logger := d.logger.Named("tcp")
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		err := conn.SetReadDeadline(time.Now().Add(time.Second * 5))
		if err != nil {
			logger.Error("failure to set read deadline on connection", "error", err)
			return
		}

		// Read in the size of the incoming packet to allocate enough mem to handle it
		var size uint16
		err = binary.Read(conn, binary.BigEndian, &size)
		if err != nil {
			if err == io.EOF {
				logger.Debug("ending connection after EOF", "error", err)
			} else {
				logger.Error("failure to read", "error", err)
			}
			return
		}

		logger.Debug("request from remote addr received", "remote_addr",
			conn.RemoteAddr().String(), "local_addr", conn.LocalAddr().String())

		logger.Debug("total data length of tcp dns request", "size", size)
		// Now that we know how much space we need, allocate a byte array to read the
		// remaining data in.
		data := make([]byte, size)
		_, err = io.ReadFull(conn, data)
		if err != nil {
			logger.Error("error reading full tcp dns request ", "error", err)
			// We can try reading it again but if this is a read timeout we don't necessarily want
			// to close the connection
			continue
		}

		logger.Debug("triaging dns request", "partition", d.partition, "namespace", d.namespace)
		responseMsg, err := d.triageAndResolve(data, pbdns.Protocol_PROTOCOL_TCP)
		if err != nil {
			logger.Error("error resolving dns request", "error", err)
			return
		}
		logger.Debug("total data length of dns response", "size", len(responseMsg))

		// This is a guard and shouldn't happen but if the response is > 65535
		// then we will just close the connection.
		if len(responseMsg) > math.MaxUint16 {
			logger.Error("dns response too large for DNS spec")
			return
		}

		// TCP DNS requests add a two byte length field prefixed to the message.
		// Source: RFC1035 4.2.2.
		err = binary.Write(conn, binary.BigEndian, uint16(len(responseMsg)))
		if err != nil {
			logger.Warn("error writing length", "error", err)
			return
		}
		_, err = conn.Write(responseMsg)
		if err != nil {
			logger.Error("error writing response", "error", err)
			return
		}
	}
}

// Stop will shut down the server
func (d *DNSServer) Stop() {
	d.lock.Lock()
	defer d.lock.Unlock()
	if !d.running {
		return
	}
	d.cancel()
}

// -----------------------------------------------------------------------------
// DNS Triage Logic
// -----------------------------------------------------------------------------

// domainClass categorises an incoming DNS query domain name.
type domainClass int

const (
	domainClassVirtual  domainClass = iota // *.virtual.*consul
	domainClassConsul                      // any other *.consul domain
	domainClassExternal                    // everything else
)

// classifyDomain returns the class of the provided DNS query name.
// The name is normalized to lower-case and any trailing '.' is removed.
//
// A name is domainClassVirtual only when it strictly matches one of the
// Consul virtual-service query forms understood by parseVirtualTokens.
// A plain *.consul name that merely contains the substring ".virtual." — such
// as "blue.virtual.service.consul" (tag "blue", service "virtual") — is
// classified as domainClassConsul and forwarded to the Consul server unchanged.
func classifyDomain(name string) domainClass {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	if strings.HasSuffix(name, ".consul") || name == "consul" {
		if _, ok := parseVirtualTokens(name); ok {
			return domainClassVirtual
		}
		return domainClassConsul
	}
	return domainClassExternal
}

// virtualTokens holds the labels of a virtual domain name. Labels absent from
// the query are empty strings; callers decide how to fill them.
type virtualTokens struct {
	port      string
	svc       string
	peer      string
	ns        string
	partition string
	dc        string
}

// parseVirtualTokens splits a virtual domain name into its service name, the
// optional named port, and any explicitly supplied peer/namespace/partition/
// datacenter tokens. ok is false when name is not a valid virtual domain.
//
// Accepted forms (all ending in ".consul"):
//
//	[<port>.]<svc>.virtual.consul
//	[<port>.]<svc>.virtual[.<val>.<qualifier>]....consul
//
// where each <qualifier> is one of "peer", "ns", "ap" or "dc", e.g.
// "http.api.virtual.peer-a.peer.default.ns.consul".
//
// The optional leading <port> label is the Consul named-port virtual DNS form
// (e.g. "http.api.virtual.consul" → service "api", port "http"). Each named
// port has its own virtual IP and its own key in Envoy's inline DNS table.
//
// Any remainder label after ".virtual." that is not a recognised
// <value>.<qualifier> pair causes ok=false so that names such as
// "blue.virtual.service.consul" (where "service" is the Consul query-kind, not
// a virtual qualifier) are not misclassified.
func parseVirtualTokens(name string) (virtualTokens, bool) {
	name = strings.ToLower(strings.TrimSuffix(name, "."))

	// Find the service name: everything up to the first ".virtual." segment.
	virtualIdx := strings.Index(name, ".virtual.")
	if virtualIdx < 0 {
		return virtualTokens{}, false
	}
	prefix := name[:virtualIdx]
	if prefix == "" {
		return virtualTokens{}, false
	}

	var t virtualTokens

	// The port is a single label (service names and port names never contain
	// dots), so a dot in the prefix means exactly "<port>.<svc>". More than one
	// dot would be an unsupported multi-label prefix.
	if dotIdx := strings.Index(prefix, "."); dotIdx >= 0 {
		if dotIdx == 0 || strings.Contains(prefix[dotIdx+1:], ".") {
			return virtualTokens{}, false
		}
		t.port = prefix[:dotIdx]
		prefix = prefix[dotIdx+1:]
	}
	t.svc = prefix
	if t.svc == "" {
		return virtualTokens{}, false
	}

	remainder := name[virtualIdx+len(".virtual."):]
	if remainder == "consul" {
		return t, true
	}
	const consulSuffix = ".consul"
	if !strings.HasSuffix(remainder, consulSuffix) {
		return virtualTokens{}, false
	}
	remainder = remainder[:len(remainder)-len(consulSuffix)]

	parts := strings.Split(remainder, ".")
	if len(parts)%2 != 0 {
		// An odd number of labels cannot form complete <val>.<qualifier> pairs.
		return virtualTokens{}, false
	}
	for i := 0; i < len(parts); i += 2 {
		val, qualifier := parts[i], parts[i+1]
		switch qualifier {
		case "peer":
			t.peer = val
		case "ns":
			t.ns = val
		case "ap":
			t.partition = val
		case "dc":
			t.dc = val
		default:
			return virtualTokens{}, false
		}
	}

	return t, true
}

// expandVirtualName expands a short-form virtual domain name into the key
// Envoy's inline DNS table uses for that upstream identity:
//
//	[<port>.]<svc>.virtual.[<peer>.peer.]<ns>.ns.<partition>.ap.<dc>.dc.consul
//
// Missing namespace/partition/datacenter tokens are filled from the source
// proxy's own defaults, matching how the Consul server resolves them. The port
// and peer labels are kept only when present in the query, mirroring the
// control plane, which omits them for local and single-port services.
func (d *DNSServer) expandVirtualName(name string) string {
	t, ok := parseVirtualTokens(name)
	if !ok {
		return strings.ToLower(strings.TrimSuffix(name, "."))
	}

	if t.ns == "" {
		t.ns = d.namespace
	}
	if t.partition == "" {
		t.partition = d.partition
	}
	if t.dc == "" {
		t.dc = d.datacenter
	}

	var b strings.Builder
	if t.port != "" {
		b.WriteString(t.port + ".")
	}
	b.WriteString(t.svc + ".virtual.")
	if t.peer != "" {
		b.WriteString(t.peer + ".peer.")
	}
	fmt.Fprintf(&b, "%s.ns.%s.ap.%s.dc.consul", t.ns, t.partition, t.dc)
	return b.String()
}

// isSidecarQueryType reports whether Envoy's DNS listeners can answer a query
// of type qt. The inline table is built only from address lists, so only A and
// AAAA can ever hit; every other type (SRV, TXT, ...) goes straight to Consul
// rather than paying an Envoy round trip that is guaranteed to miss.
func isSidecarQueryType(qt dnsmessage.Type) bool {
	return qt == dnsmessage.TypeA || qt == dnsmessage.TypeAAAA
}

// rewriteQueryName rewrites the question section of a DNS message with
// newName and returns the modified raw bytes.  The original name in the
// question is replaced in-place at the wire level by re-encoding it.
func rewriteQueryName(raw []byte, newName string) ([]byte, string, error) {
	var msg dnsmessage.Message
	if err := msg.Unpack(raw); err != nil {
		return nil, "", fmt.Errorf("unpack dns message: %w", err)
	}
	if len(msg.Questions) == 0 {
		return raw, "", nil
	}
	originalName := msg.Questions[0].Name.String()
	fqdn := newName
	if !strings.HasSuffix(fqdn, ".") {
		fqdn += "."
	}
	name, err := dnsmessage.NewName(fqdn)
	if err != nil {
		return nil, "", fmt.Errorf("build dns name %q: %w", fqdn, err)
	}
	msg.Questions[0].Name = name
	// Re-pack the message to re-encode the updated QNAME.
	rewritten, err := msg.Pack()
	if err != nil {
		return nil, "", fmt.Errorf("pack dns message: %w", err)
	}
	return rewritten, originalName, nil
}

// rewriteResponseName replaces all occurrences of expandedName in the
// response's answer/authority/additional sections with originalName so the
// caller receives a response that matches the name it queried.
func rewriteResponseName(raw []byte, expandedName, originalName string) ([]byte, error) {
	var msg dnsmessage.Message
	if err := msg.Unpack(raw); err != nil {
		return raw, nil // best-effort; return original on parse failure
	}
	expanded, err := dnsmessage.NewName(canonicalName(expandedName))
	if err != nil {
		return raw, nil
	}
	original, err := dnsmessage.NewName(canonicalName(originalName))
	if err != nil {
		return raw, nil
	}

	rewrite := func(name *dnsmessage.Name) {
		if name.String() == expanded.String() {
			*name = original
		}
	}
	for i := range msg.Questions {
		rewrite(&msg.Questions[i].Name)
	}
	for i := range msg.Answers {
		rewrite(&msg.Answers[i].Header.Name)
	}
	for i := range msg.Authorities {
		rewrite(&msg.Authorities[i].Header.Name)
	}
	for i := range msg.Additionals {
		rewrite(&msg.Additionals[i].Header.Name)
	}
	out, err := msg.Pack()
	if err != nil {
		return raw, nil
	}
	return out, nil
}

func canonicalName(n string) string {
	n = strings.ToLower(n)
	if !strings.HasSuffix(n, ".") {
		n += "."
	}
	return n
}

// isEnvoyHit reports whether an Envoy inline-listener response should be
// returned directly to the client. Only RCodeSuccess is treated as a hit;
// every other rcode (NXDOMAIN, FORMERR, SERVFAIL, NOTIMP, REFUSED, …) causes
// the caller to fall back to the Consul server.
func isEnvoyHit(raw []byte) bool {
	var msg dnsmessage.Message
	if err := msg.Unpack(raw); err != nil {
		return false
	}
	return msg.RCode == dnsmessage.RCodeSuccess
}

// dnsAnswerCount returns the number of resource records in a raw DNS
// response's answer section, or 0 if the message can't be parsed.
func dnsAnswerCount(raw []byte) int {
	var msg dnsmessage.Message
	if err := msg.Unpack(raw); err != nil {
		return 0
	}
	return len(msg.Answers)
}

// forwardUDP sends a raw DNS query to addr and returns the raw response.
func forwardUDP(addr string, query []byte, timeout time.Duration) ([]byte, error) {
	conn, err := net.DialTimeout("udp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("dial udp %s: %w", addr, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	if _, err := conn.Write(query); err != nil {
		return nil, fmt.Errorf("write udp query: %w", err)
	}
	buf := make([]byte, 65535)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("read udp response: %w", err)
	}
	return buf[:n], nil
}

// queryConsul forwards a raw DNS message to the Consul server via gRPC and
// returns the raw response bytes.
func (d *DNSServer) queryConsul(raw []byte, proto pbdns.Protocol) ([]byte, error) {
	req := &pbdns.QueryRequest{Msg: raw, Protocol: proto}
	ctx, cancel := context.WithTimeout(context.Background(), consulQueryTimeout)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx,
		"x-consul-partition", d.partition,
		"x-consul-namespace", d.namespace,
		"x-consul-token", d.token,
	)
	resp, err := d.client.Query(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func (d *DNSServer) canTryInlineListener() bool {
	return time.Since(processStart) > time.Duration(d.inlineListenerUnavailableTill.Load())
}

func (d *DNSServer) canTryEgressListener() bool {
	return time.Since(processStart) > time.Duration(d.egressListenerUnavailableTill.Load())
}

func (d *DNSServer) markInlineListenerUnavailable() {
	advanceDeadline(&d.inlineListenerUnavailableTill, int64(time.Since(processStart)+inlineListenerUnhealthyTTL))
}

func (d *DNSServer) markEgressListenerUnavailable() {
	advanceDeadline(&d.egressListenerUnavailableTill, int64(time.Since(processStart)+listenerUnhealthyTTL))
}

// advanceDeadline moves deadline forward to newVal, but only if newVal is
// later than the currently stored value. This is done via a CAS loop rather
// than a plain Store so that concurrent callers can't clobber a later
// deadline with an earlier one (e.g. a goroutine that computed its deadline
// and then got descheduled before storing it), which would otherwise shorten
// the intended backoff.
func advanceDeadline(deadline *atomic.Int64, newVal int64) {
	for {
		cur := deadline.Load()
		if newVal <= cur {
			return
		}
		if deadline.CompareAndSwap(cur, newVal) {
			return
		}
	}
}

// triageAndResolve is the main entry point for the virtual DNS triage logic.
// It classifies the domain, expands FQDNs, forwards to the right backend, and
// handles NXDOMAIN fallback.  proto determines which protocol label is used for
// Consul gRPC queries.
func (d *DNSServer) triageAndResolve(raw []byte, proto pbdns.Protocol) ([]byte, error) {
	// Parse domain from the first question.
	var msg dnsmessage.Message
	if err := msg.Unpack(raw); err != nil || len(msg.Questions) == 0 {
		// Unparseable — fall back to Consul server.
		return d.queryConsul(raw, proto)
	}
	originalName := strings.TrimSuffix(msg.Questions[0].Name.String(), ".")

	class := classifyDomain(originalName)

	if class == domainClassVirtual && !isSidecarQueryType(msg.Questions[0].Type) {
		d.logger.Debug("query type not served by sidecar dns listeners, querying consul directly",
			"domain", originalName, "qtype", msg.Questions[0].Type)
		return d.queryConsul(raw, proto)
	}

	switch class {
	case domainClassConsul:
		// Standard Consul domain — unchanged path.
		return d.queryConsul(raw, proto)

	case domainClassExternal:
		// Non-consul domain.
		d.logger.Debug("external dns query, routing to egress listener", "domain", originalName)
		if d.datacenter == "" || d.virtualDNSEgressAddr == "" || !d.canTryEgressListener() {
			return d.queryConsul(raw, proto)
		}

		// Forward to Envoy egress DNS listener (UDP only — c-ares plugin is UDP).
		resp, err := forwardUDP(d.virtualDNSEgressAddr, raw, envoyDNSForwardTimeout)
		if err != nil {
			d.markEgressListenerUnavailable()
			d.logger.Debug("egress listener unavailable, falling back to consul", "error", err)
			return d.queryConsul(raw, proto)
		}
		if !isEnvoyHit(resp) {
			// A non-success rcode (e.g. NXDOMAIN) is a normal answer from a healthy
			// listener, so fall back for this query without backing off.
			d.logger.Debug("egress listener returned non-success rcode, falling back to consul", "domain", originalName)
			return d.queryConsul(raw, proto)
		}
		d.logger.Debug("external dns resolved via egress listener", "domain", originalName)
		return resp, nil

	case domainClassVirtual:
		if d.datacenter == "" || d.virtualDNSInlineAddr == "" {
			return d.queryConsul(raw, proto)
		}

		// The Consul server rejects a virtual query naming both a peer and a
		// datacenter. Let it produce that answer rather than letting the inline
		// table answer a query the server would refuse.
		if t, ok := parseVirtualTokens(originalName); ok && t.peer != "" && t.dc != "" {
			return d.queryConsul(raw, proto)
		}

		// Expand the short form to the full FQDN.
		expandedName := d.expandVirtualName(originalName)

		// Rewrite the query with the expanded name before forwarding to Envoy.
		rewrittenQuery, _, err := rewriteQueryName(raw, expandedName)
		d.logger.Debug("virtual dns query", "original_name", originalName, "expanded_name", expandedName, "rewritten_query_len", len(rewrittenQuery), "error", err)
		if err != nil {
			d.logger.Warn("failed to rewrite query name, falling back to consul", "error", err)
			return d.queryConsul(raw, proto)
		}

		// Forward to Envoy's inline DNS listener (UDP — dns_filter is UDP-only).
		if !d.canTryInlineListener() {
			envoyErr := errors.New("inline listener in backoff window")
			d.logger.Debug("virtual dns inline listener in backoff, falling back to consul", "domain", originalName, "error", envoyErr)
			return d.queryConsul(raw, proto)
		}

		envoyResp, envoyErr := forwardUDP(d.virtualDNSInlineAddr, rewrittenQuery, envoyDNSForwardTimeout)
		if envoyErr == nil && isEnvoyHit(envoyResp) {
			if n := dnsAnswerCount(envoyResp); n > 1 {
				// Each upstream identity has its own inline-table key with a
				// single VIP, so more than one answer means the table merged
				// identities the FQDN does not distinguish (e.g. an older
				// control plane without port/peer labels). Envoy itself can't
				// disambiguate which address belongs to the identity this
				// query actually means, so trusting this answer risks
				// silently routing to the wrong one. Defer to the real
				// Consul server instead, which resolves the query using the
				// caller's true identity context rather than a collapsed
				// inline table. Note this deliberately falls through to the
				// same Consul fallback used for an Envoy miss/error below.
				d.logger.Debug("virtual dns inline listener returned an ambiguous multi-answer response, falling back to consul",
					"domain", originalName, "expanded_name", expandedName, "answer_count", n)
			} else {
				// Hit — rewrite the response name back to the original and return.
				d.logger.Debug("virtual dns resolved via inline listener", "domain", originalName, "expanded_name", expandedName)
				out, err := rewriteResponseName(envoyResp, expandedName, originalName)
				if err != nil {
					return envoyResp, nil
				}
				return out, nil
			}
		}
		if envoyErr != nil {
			d.markInlineListenerUnavailable()
		}

		// NXDOMAIN or an ambiguous multi-answer response from Envoy (or a
		// forwarding error) — fall back to Consul server. Use the original
		// (unexpanded) query so the server applies its own expansion logic.
		d.logger.Debug("virtual dns miss, falling back to consul server",
			"domain", originalName, "envoy_error", envoyErr)
		consulResp, consulErr := d.queryConsul(raw, proto)
		if consulErr != nil {
			if envoyErr != nil {
				return nil, fmt.Errorf("virtual dns: envoy error: %v; consul fallback error: %w", envoyErr, consulErr)
			}
			return nil, consulErr
		}
		return consulResp, nil
	}

	// Should never reach here.
	return d.queryConsul(raw, proto)
}
