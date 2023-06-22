// Copyright (c) HashiCorp, Inc.
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
	"sync"
	"time"

	"github.com/hashicorp/consul/proto-public/pbdns"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc/metadata"
)

// ErrServerDisabled is returned when the server is disabled
var ErrServerDisabled error = errors.New("server is disabled")

// ErrServerRunning is returned when the server is already running
var ErrServerRunning error = errors.New("server is already running")

// DNSServerParams is the configuration for creating a new DNS server
type DNSServerParams struct {
	BindAddr string
	Port     int
	Logger   hclog.Logger
	Client   pbdns.DNSServiceClient

	Partition string
	Namespace string
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

	partition string
	namespace string
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
	s.namespace = p.Namespace
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
	req := &pbdns.QueryRequest{
		Msg:      buf,
		Protocol: pbdns.Protocol_PROTOCOL_UDP,
	}

	ctx, done := context.WithTimeout(context.Background(), time.Minute*1)
	defer done()

	ctx = metadata.AppendToOutgoingContext(ctx,
		"x-consul-partition", d.partition,
		"x-consul-namespace", d.namespace,
	)

	resp, err := d.client.Query(ctx, req)
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

		// Now that we have the request we can forward the dnsrequest to consul
		req := &pbdns.QueryRequest{
			Msg:      data,
			Protocol: pbdns.Protocol_PROTOCOL_TCP,
		}

		ctx, done := context.WithTimeout(context.Background(), time.Minute*1)
		defer done()

		resp, err := client.Query(ctx, req)
		if err != nil {
			logger.Error("error resolving consul request", "error", err)
			return
		}
		logger.Debug("total data length of dns response from consul", "size", len(resp.Msg))

		// This is a guard and shouldn't happen but if the response is > 65535
		// then we will just close the connection.
		if len(resp.Msg) > math.MaxUint16 {
			logger.Error("consul response too large for DNS spec", "error", err)
			return
		}

		// TCP DNS requests add a two byte length field prefixed to the message.
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
	d.lock.Lock()
	defer d.lock.Unlock()
	if !d.running {
		return
	}
	d.cancel()
}
