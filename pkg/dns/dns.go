package dns

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"time"

	"github.com/hashicorp/consul/proto-public/pbdns"
	"github.com/hashicorp/go-hclog"
)

// ErrServerDisabled is returned when the server is disabled
var ErrServerDisabled error = errors.New("server is disabled")

// DNSServerParams is the configuration for creating a new DNS server
type DNSServerParams struct {
	BindAddr string
	Port     int
	Logger   hclog.Logger
	Client   pbdns.DNSServiceClient
}

// DNSServerInterface is the interface for athe DNSServer
type DNSServerInterface interface {
	Run() error
	Stop()
	TcpPort() int
	UdpPort() int
}

// DNSServer is the implementation of the DNSServerInterface
type DNSServer struct {
	bindAddr net.IP
	port     int

	logger      hclog.Logger
	client      pbdns.DNSServiceClient
	connUDP     net.PacketConn
	listenerTCP net.Listener
	stopCh      chan (struct{})
}

// NewDNSServer creates a new DNS proxy server
func NewDNSServer(p DNSServerParams) (DNSServerInterface, error) {
	s := &DNSServer{}
	s.bindAddr = net.ParseIP(p.BindAddr)
	if s.bindAddr == nil {
		return nil, fmt.Errorf("error parsing specified dns bind addr '%s'", p.BindAddr)
	}
	s.port = p.Port
	s.client = p.Client
	s.logger = p.Logger.Named("dns-proxy")
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
func (d *DNSServer) Run() error {
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
		return fmt.Errorf("error listening to udp: %w", err)
	}
	d.listenerTCP = listenerTCP

	// 3. Start go routines to handle dns proxying
	go d.proxyUDP()
	go d.proxyTCP()

	// 4. Create stop channel for stopping server
	d.stopCh = make(chan struct{})

	d.logger.Info("running dns proxy", " udp port", d.UdpPort(), "tcp port", d.TcpPort())
	return nil
}

func (d *DNSServer) proxyUDP() {
	logger := d.logger.Named("udp")
	for {
		select {
		case <-d.stopCh:
			d.connUDP.Close()
			return
		default:
		}
		buf := make([]byte, 512)
		err := d.connUDP.SetReadDeadline(time.Now().Add(time.Second * 10))
		if err != nil {
			logger.Error("failure to set read deadline on connection", "error", err)
			return
		}
		bytesRead, addr, err := d.connUDP.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				logger.Info("connection closed")
				break
			} else {
				if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
					logger.Debug("timeout waiting for read", "error", err)
				} else {
					logger.Warn("error reading from conn", "error", err)
				}
				continue
			}
		}
		// Parallelize responses
		go d.queryConsulAndRespondUDP(buf[0:bytesRead], addr)
	}
}

func (d *DNSServer) queryConsulAndRespondUDP(buf []byte, addr net.Addr) {
	logger := d.logger.Named("udp")
	req := &pbdns.QueryRequest{
		Msg:      buf,
		Protocol: pbdns.Protocol_PROTOCOL_UDP,
	}

	resp, err := d.client.Query(context.Background(), req)
	if err != nil {
		logger.Error("error resolving consul request", "error", err)
		return
	}
	logger.Debug("dns messaged received from consul", "length", len(resp.Msg))
	_, err = d.connUDP.WriteTo(resp.Msg, addr)
	if err != nil {
		logger.Error("error sending response", "error", err)
		return
	}
}

func (d *DNSServer) proxyTCP() {
	defer d.listenerTCP.Close()
	for {
		select {
		case <-d.stopCh:
			return
		default:
		}
		c, err := d.listenerTCP.Accept()
		if err != nil {
			d.logger.Warn("failure to accept tcp connection", "error", err)
		}
		go d.proxyTCPAcceptedConn(c, d.client)
	}
}

func (d *DNSServer) proxyTCPAcceptedConn(conn net.Conn, client pbdns.DNSServiceClient) {
	defer conn.Close()
	logger := d.logger.Named("tcp")
	for {
		select {
		case <-d.stopCh:
			return
		default:
		}
		err := conn.SetReadDeadline(time.Now().Add(time.Second * 5))
		if err != nil {
			logger.Error("failure to set read deadline on connection", "error", err)
			return
		}

		// Read in the size of the incoming packet to allocate enough mem to handle it
		var prefixSize uint16
		err = binary.Read(conn, binary.BigEndian, &prefixSize)
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

		size := prefixSize
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

		// Now that we have the request we can forward the dnsrequest to consul
		req := &pbdns.QueryRequest{
			Msg:      data,
			Protocol: pbdns.Protocol_PROTOCOL_TCP,
		}
		resp, err := client.Query(context.Background(), req)
		if err != nil {
			logger.Error("error resolving consul request", "error", err)
			return
		}
		logger.Debug("total data length of dns response from consul", "size", len(resp.Msg))

		// This is a guard and shouldn't happen but if the response is > 65535
		// then we will just close the connection.
		// Source: RFC1035 4.2.2.
		if len(resp.Msg) > math.MaxUint16 {
			logger.Error("consul response too large for DNS spec", "error", err)
			return
		}

		// TCP DNS requests allcate a two byte length field prefixed to the message.
		// Source: RFC1035 4.2.2.
		err = binary.Write(conn, binary.BigEndian, uint16(len(resp.Msg)))
		if err != nil {
			logger.Warn("error writing length", "error", err)
			return
		}
		_, err = conn.Write(resp.Msg)
		if err != nil {
			logger.Error("error writing response", "error", err)
			return
		}
	}
}

// Stop will shut down the server
func (d *DNSServer) Stop() {
	if d.stopCh != nil {
		close(d.stopCh)
	}
}
