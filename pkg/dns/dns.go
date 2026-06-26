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
	"time"

	"github.com/hashicorp/consul/proto-public/pbdns"
	"github.com/hashicorp/go-hclog"
	"golang.org/x/net/dns/dnsmessage"
	"google.golang.org/grpc/metadata"
)

// ErrServerDisabled is returned when the server is disabled
var ErrServerDisabled error = errors.New("server is disabled")

// ErrServerRunning is returned when the server is already running
var ErrServerRunning error = errors.New("server is already running")

const (
	envoyDNSForwardTimeout = 300 * time.Millisecond
	listenerUnhealthyTTL   = 5 * time.Second
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

	listenerHealthLock            sync.Mutex
	inlineListenerUnavailableTill time.Time
	egressListenerUnavailableTill time.Time
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
			} else if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				logger.Debug("timeout waiting for read", "error", err)
			} else {
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
			d.logger.Warn("failure to accept tcp connection", "error", err)
		}
		go d.proxyTCPAcceptedConn(ctx, c, d.client)
	}
}

func (d *DNSServer) proxyTCPAcceptedConn(ctx context.Context, conn net.Conn, client pbdns.DNSServiceClient) {
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

// classifyDomain returns the class of the fully-qualified domain name.
// name must be in canonical (lower-case, trailing-dot-stripped) form.
func classifyDomain(name string) domainClass {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	if strings.HasSuffix(name, ".consul") || name == "consul" {
		// Check for virtual: must contain ".virtual." segment
		if strings.Contains(name, ".virtual.") {
			return domainClassVirtual
		}
		return domainClassConsul
	}
	return domainClassExternal
}

// expandVirtualFQDN expands any of the 8 short-form virtual domain names into
// the canonical form:
//
//	<svc>.virtual.<ns>.ns.<partition>.ap.<dc>.dc.consul
//
// Missing components are filled from the server's own namespace, partition and
// datacenter.
func expandVirtualFQDN(name, defaultNS, defaultPartition, defaultDC string) string {
	name = strings.ToLower(strings.TrimSuffix(name, "."))

	// Find the service name: everything up to the first ".virtual." segment.
	virtualIdx := strings.Index(name, ".virtual.")
	if virtualIdx < 0 {
		return name
	}
	svc := name[:virtualIdx]
	// Remainder after "<svc>.virtual." — may be empty (just "consul") or have
	// ns/ap/dc qualifiers.
	remainder := name[virtualIdx+len(".virtual."):]
	// Strip trailing ".consul" if present.
	remainder = strings.TrimSuffix(remainder, ".consul")

	ns := defaultNS
	partition := defaultPartition
	dc := defaultDC

	// Parse remainder tokens separated by ".":
	// Possible patterns of (label, qualifier) pairs: ns, ap, dc in any order.
	parts := strings.Split(remainder, ".")
	for i := 0; i+1 < len(parts); i++ {
		switch parts[i+1] {
		case "ns":
			ns = parts[i]
			i++ // skip qualifier
		case "ap":
			partition = parts[i]
			i++
		case "dc":
			dc = parts[i]
			i++
		}
	}

	return fmt.Sprintf("%s.virtual.%s.ns.%s.ap.%s.dc.consul", svc, ns, partition, dc)
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
	// Also rewrite additional / answer names if present (re-use same target).
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
	expanded := dnsmessage.MustNewName(canonicalName(expandedName))
	original := dnsmessage.MustNewName(canonicalName(originalName))

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

// isNXDOMAIN returns true when the DNS response carries an NXDOMAIN rcode.
func isNXDOMAIN(raw []byte) bool {
	var msg dnsmessage.Message
	if err := msg.Unpack(raw); err != nil {
		return false
	}
	return msg.Header.RCode == dnsmessage.RCodeNameError
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
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
	d.listenerHealthLock.Lock()
	defer d.listenerHealthLock.Unlock()
	return time.Now().After(d.inlineListenerUnavailableTill)
}

func (d *DNSServer) canTryEgressListener() bool {
	d.listenerHealthLock.Lock()
	defer d.listenerHealthLock.Unlock()
	return time.Now().After(d.egressListenerUnavailableTill)
}

func (d *DNSServer) markInlineListenerUnavailable() {
	d.listenerHealthLock.Lock()
	defer d.listenerHealthLock.Unlock()
	d.inlineListenerUnavailableTill = time.Now().Add(listenerUnhealthyTTL)
}

func (d *DNSServer) markEgressListenerUnavailable() {
	d.listenerHealthLock.Lock()
	defer d.listenerHealthLock.Unlock()
	d.egressListenerUnavailableTill = time.Now().Add(listenerUnhealthyTTL)
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

	switch class {
	case domainClassConsul:
		// Standard Consul domain — unchanged path.
		return d.queryConsul(raw, proto)

	case domainClassExternal:
		// Non-consul domain.
		if !d.canTryEgressListener() {
			return d.queryConsul(raw, proto)
		}

		// Forward to Envoy egress DNS listener (UDP only — c-ares plugin is UDP).
		resp, err := forwardUDP(d.virtualDNSEgressAddr, raw, envoyDNSForwardTimeout)
		if err != nil {
			d.markEgressListenerUnavailable()
			d.logger.Debug("egress listener unavailable, falling back to consul", "error", err)
			return d.queryConsul(raw, proto)
		}
		return resp, nil

	case domainClassVirtual:
		// Expand the short form to the full FQDN.
		expandedName := expandVirtualFQDN(originalName, d.namespace, d.partition, d.datacenter)

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
		if envoyErr == nil && !isNXDOMAIN(envoyResp) {
			// Hit — rewrite the response name back to the original and return.
			out, err := rewriteResponseName(envoyResp, expandedName, originalName)
			if err != nil {
				return envoyResp, nil
			}
			return out, nil
		}
		if envoyErr != nil {
			d.markInlineListenerUnavailable()
		}

		// NXDOMAIN from Envoy (or forwarding error) — fall back to Consul server.
		// Use the original (unexpanded) query so the server applies its own
		// expansion logic.
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
