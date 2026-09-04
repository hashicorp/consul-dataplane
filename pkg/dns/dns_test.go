// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package dns

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/hashicorp/consul/proto-public/pbdns"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/dns/dnsmessage"
	"google.golang.org/grpc/metadata"

	"github.com/hashicorp/consul-dataplane/pkg/dns/mocks"
)

// timeoutReadErr implements net.Error with Timeout() == true, simulating the
// benign idle read-deadline error that the UDP proxy loop should swallow
// silently (no "timeout waiting for read" log, no read-error warning).
type timeoutReadErr string

func (e timeoutReadErr) Error() string { return string(e) }
func (timeoutReadErr) Timeout() bool   { return true }
func (timeoutReadErr) Temporary() bool { return true }

type MockedNetConn struct {
	net.Conn
	mock.Mock
}

type mockedPacketConn struct {
	mock.Mock
}

func (m *mockedPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	args := m.Called(p)
	addr, _ := args.Get(1).(net.Addr)
	return args.Int(0), addr, args.Error(2)
}

func (m *mockedPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	args := m.Called(p, addr)
	return args.Int(0), args.Error(1)
}

func (m *mockedPacketConn) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *mockedPacketConn) LocalAddr() net.Addr {
	args := m.Called()
	addr, _ := args.Get(0).(net.Addr)
	return addr
}

func (m *mockedPacketConn) SetDeadline(t time.Time) error {
	args := m.Called(t)
	return args.Error(0)
}

func (m *mockedPacketConn) SetReadDeadline(t time.Time) error {
	args := m.Called(t)
	return args.Error(0)
}

func (m *mockedPacketConn) SetWriteDeadline(t time.Time) error {
	args := m.Called(t)
	return args.Error(0)
}

type DNSTestSuite struct {
	suite.Suite
}

func TestDNS_suite(t *testing.T) {
	suite.Run(t, new(DNSTestSuite))
}

func genRandomBytes(size int) (blk []byte) {
	blk = make([]byte, size)
	_, _ = rand.Read(blk)
	return blk
}

func (s *DNSTestSuite) Test_DisabledServer() {
	mockedDNSConsulClient := mocks.NewDNSServiceClient(s.T())
	server, err := NewDNSServer(DNSServerParams{
		BindAddr: "127.0.0.1",
		Port:     -1, // disabled server
		Logger:   hclog.Default(),
		Client:   mockedDNSConsulClient,
	})
	s.Require().Equal(ErrServerDisabled, err)
	s.Require().Nil(server)

	// Not really necessary but covers the case where we somehow have a server without
	// a tcp conn or udp conn initialized.
	sv := &DNSServer{
		client: mockedDNSConsulClient,
		logger: hclog.Default(),
	}
	s.Require().Equal(sv.TcpPort(), -1)
	s.Require().Equal(sv.UdpPort(), -1)

}

func (s *DNSTestSuite) Test_AlreadyRunning() {
	mockedDNSConsulClient := mocks.NewDNSServiceClient(s.T())
	server, err := NewDNSServer(DNSServerParams{
		BindAddr: "127.0.0.1",
		Port:     0, // disabled server
		Logger:   hclog.Default(),
		Client:   mockedDNSConsulClient,
	})
	if err != nil {
		s.T().FailNow()
	}
	err = server.Start(context.Background())
	defer server.Stop()
	s.Require().NoError(err)
	err = server.Start(context.Background())
	s.Require().Error(err)
	s.Require().ErrorIs(err, ErrServerRunning)
}

func (s *DNSTestSuite) Test_ServerStop() {
	mockedDNSConsulClient := mocks.NewDNSServiceClient(s.T())
	server, err := NewDNSServer(DNSServerParams{
		BindAddr: "127.0.0.1",
		Port:     0, // let the os choose a port
		Logger:   hclog.Default(),
		Client:   mockedDNSConsulClient,
	})
	if err != nil {
		s.T().FailNow()
	}

	err = server.Start(context.Background())
	if err != nil {
		s.T().FailNow()
	}
	tcpport := server.TcpPort()
	udpport := server.UdpPort()
	server.Stop()

	s.Require().Eventually(func() bool {

		addr := fmt.Sprintf("127.0.0.1:%v", tcpport)
		_, err := net.Dial("tcp", addr)
		s.T().Logf("dial error: %v", err)
		return err != nil
	}, time.Second*5, time.Second, "Failure to shut down tcp")

	s.Require().Eventually(func() bool {
		addr := fmt.Sprintf("127.0.0.1:%v", udpport)
		c, _ := net.Dial("udp", addr)
		_, _ = c.Write([]byte("here"))
		p := make([]byte, 512)
		_, err = c.Read(p)
		s.T().Logf("read udp error: %v", err)
		return err != nil
	}, time.Second*5, time.Second, "Failure to shut down udp")
}

func (s *DNSTestSuite) Test_UDPProxy() {
	mockedDNSConsulClient := mocks.NewDNSServiceClient(s.T())
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
	connUdp, err := net.ListenUDP("udp", addr)
	s.Require().NoError(err)
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := DNSServer{
		client:    mockedDNSConsulClient,
		connUDP:   connUdp,
		logger:    hclog.Default(),
		partition: "test-partition",
		namespace: "test-namespace",
		token:     "test-token",
	}

	go server.proxyUDP(runCtx)

	testCases := map[string]struct {
		dnsRequest   []byte
		dnsResp      []byte
		expected     error
		expectedGRPC error
	}{

		"happy path": {
			dnsRequest: genRandomBytes(512),
			dnsResp:    genRandomBytes(50),
		},
		"happy large response path": {
			dnsRequest: genRandomBytes(50),
			dnsResp:    genRandomBytes(9216), // net.inet.udp.maxdgram for macs
		},
		"bad consul response too large": {
			dnsRequest: genRandomBytes(50),
			dnsResp:    genRandomBytes(65536),
			expected:   errors.New("timeout"),
		},
		"bad consul response": {
			dnsRequest:   genRandomBytes(512),
			dnsResp:      genRandomBytes(50),
			expectedGRPC: errors.New("timeout"),
		},
	}

	for name, tc := range testCases {
		s.Run(name, func() {

			req := tc.dnsRequest
			resp := tc.dnsResp

			clientResp := &pbdns.QueryResponse{
				Msg: resp,
			}

			mockedDNSConsulClient.On("Query", mock.Anything, mock.Anything).
				Run(func(args mock.Arguments) {
					ctx, ok := args.Get(0).(context.Context)
					require.True(s.T(), ok, "error casting to context.Context")

					md, ok := metadata.FromOutgoingContext(ctx)
					require.True(s.T(), ok, "error getting metadata from context")

					require.Equal(s.T(), "test-token", md.Get("x-consul-token")[0], "token not set in context")
					require.Equal(s.T(), "test-namespace", md.Get("x-consul-namespace")[0], "namespace not set in context")
					require.Equal(s.T(), "test-partition", md.Get("x-consul-partition")[0], "partition not set in context")
				}).
				Return(clientResp, tc.expectedGRPC).Once()
			addr := fmt.Sprintf("127.0.0.1:%v", server.UdpPort())

			conn, err := net.Dial("udp", addr)

			s.Require().NoError(err)

			n, err := conn.Write(req)
			if err != nil {
				s.T().Logf("error: %v", err.Error())
			}
			s.T().Logf("written %v", n)
			p := make([]byte, 9216)
			_ = conn.SetReadDeadline(time.Now().Add(time.Second * 1))
			lengthRead, err := conn.Read(p)
			s.T().Logf("read %v", lengthRead)
			if tc.expectedGRPC != nil {
				s.Require().Error(err)
				s.Require().ErrorContains(err, tc.expectedGRPC.Error())
			} else if tc.expected != nil {
				s.Require().Error(err)
				s.Require().ErrorContains(err, tc.expected.Error())
				return
			} else {
				s.Require().NoError(err, "exchange error")
				s.Require().EqualValues(resp, p[0:lengthRead])
				s.Require().Equal(lengthRead, len(resp))
			}
			conn.Close()
		})
	}

}

func (s *DNSTestSuite) Test_UDPProxy_ReadErrorLogging() {
	testCases := map[string]struct {
		readErr error
		// wantLogged, if true, asserts the read-error warning IS logged
		// (non-timeout errors). If false, asserts that neither the warning
		// nor the timeout-specific message are ever logged (timeout errors
		// must stay silent).
		wantLogged bool
	}{
		"net error": {
			readErr:    &net.OpError{Err: errors.New("read failed")},
			wantLogged: true,
		},
		"non-net error": {
			readErr:    errors.New("read failed"),
			wantLogged: true,
		},
		"timeout error": {
			readErr:    timeoutReadErr("read udp 127.0.0.1:1053: i/o timeout"),
			wantLogged: false,
		},
	}

	for name, tc := range testCases {
		s.Run(name, func() {
			var logBuf bytes.Buffer

			mockedDNSConsulClient := mocks.NewDNSServiceClient(s.T())
			connUDP := &mockedPacketConn{}
			runCtx, cancel := context.WithCancel(context.Background())
			defer cancel()

			server := DNSServer{
				client:  mockedDNSConsulClient,
				connUDP: connUDP,
				logger: hclog.New(&hclog.LoggerOptions{
					Level:  hclog.Debug,
					Output: &logBuf,
				}),
			}

			connUDP.On("SetReadDeadline", mock.Anything).Return(nil).Once()
			connUDP.On("ReadFrom", mock.Anything).Run(func(args mock.Arguments) {
				cancel()
			}).Return(0, (*net.UDPAddr)(nil), tc.readErr).Once()
			connUDP.On("Close").Return(nil).Once()

			server.proxyUDP(runCtx)

			connUDP.AssertExpectations(s.T())
			mockedDNSConsulClient.AssertNotCalled(s.T(), "Query", mock.Anything, mock.Anything)

			logged := logBuf.String()
			if tc.wantLogged {
				s.Require().Contains(logged, "error reading from conn", "expected non-timeout read error to be logged")
			} else {
				s.Require().NotContains(logged, "timeout waiting for read", "timeout errors must not be logged with this message")
				s.Require().NotContains(logged, "error reading from conn", "timeout errors must not trigger the read-error warning")
			}
		})
	}
}

func (s *DNSTestSuite) Test_ProxydnsTCP() {
	mockedDNSConsulClient := mocks.NewDNSServiceClient(s.T())
	addr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
	listenerTCP, err := net.ListenTCP("tcp", addr)
	s.Require().NoError(err)

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := DNSServer{
		client:      mockedDNSConsulClient,
		listenerTCP: listenerTCP,
		logger:      hclog.Default(),
		partition:   "test-partition",
		namespace:   "test-namespace",
		token:       "test-token",
	}

	go server.proxyTCP(runCtx)

	testCases := map[string]struct {
		dnsRequest   []byte
		dnsResp      []byte
		expected     error
		largeResp    bool
		expectedGRPC error
	}{
		"happy path": {
			dnsRequest: genRandomBytes(50),
			dnsResp:    genRandomBytes(50),
		},
		"happy path large ": {
			dnsRequest: genRandomBytes(50),
			dnsResp:    genRandomBytes(65467),
		},
		"happy path large dns": {
			dnsRequest: genRandomBytes(50),
			dnsResp:    genRandomBytes(65536),
			largeResp:  true,
		},
		"no consul server response": {
			dnsRequest:   genRandomBytes(50),
			dnsResp:      genRandomBytes(50),
			expectedGRPC: errors.New("EOF"),
		},
	}
	for name, tc := range testCases {
		s.Run(name, func() {

			req := tc.dnsRequest
			resp := tc.dnsResp

			clientResp := &pbdns.QueryResponse{
				Msg: resp,
			}

			mockedDNSConsulClient.On("Query", mock.Anything, mock.Anything).
				Run(func(args mock.Arguments) {
					ctx, ok := args.Get(0).(context.Context)
					require.True(s.T(), ok, "error casting to context.Context")

					md, ok := metadata.FromOutgoingContext(ctx)
					require.True(s.T(), ok, "error getting metadata from context")

					require.Equal(s.T(), "test-token", md.Get("x-consul-token")[0], "token not set in context")
					require.Equal(s.T(), "test-namespace", md.Get("x-consul-namespace")[0], "namespace not set in context")
					require.Equal(s.T(), "test-partition", md.Get("x-consul-partition")[0], "partition not set in context")
				}).
				Return(clientResp, tc.expectedGRPC).
				Once()
			addr := fmt.Sprintf("127.0.0.1:%v", server.TcpPort())

			conn, err := net.Dial("tcp", addr)
			s.Require().NoError(err)

			defer conn.Close()
			_ = binary.Write(conn, binary.BigEndian, uint16(len(req)))
			_, _ = conn.Write(req)

			var length uint16
			err = binary.Read(conn, binary.BigEndian, &length)
			if tc.largeResp || tc.expectedGRPC != nil {
				s.Require().Error(err)
				s.Require().ErrorContains(err, "EOF")
				return
			}
			s.Require().NoError(err)

			p := make([]byte, length)
			v, err := io.ReadFull(conn, p)

			if tc.expected != nil {
				s.Require().Error(err)
				s.Require().ErrorContains(err, tc.expected.Error())
			} else if tc.expectedGRPC != nil {
				s.Require().Error(err)
				s.Require().ErrorContains(err, "EOF")
			} else {
				s.Require().NoError(err, "exchange error")
				s.Require().EqualValues(resp, p)
				s.Require().Equal(v, len(resp))
			}
		})
	}
}

func (s *DNSTestSuite) Test_ClassifyDomain() {
	testCases := map[string]domainClass{
		// valid virtual forms
		"service.virtual.consul":                              domainClassVirtual,
		"service.virtual.default.ns.default.ap.dc1.dc.consul": domainClassVirtual,
		// collision regression: "blue" is a tag, "virtual" is the service name,
		// "service" is the Consul query-kind label — must not be domainClassVirtual.
		"blue.virtual.service.consul": domainClassConsul,
		// other plain consul domains
		"service.default.consul": domainClassConsul,
		"consul":                 domainClassConsul,
		// external
		"google.com": domainClassExternal,
	}

	for domain, expected := range testCases {
		s.Run(domain, func() {
			s.Require().Equal(expected, classifyDomain(domain))
		})
	}
}

func (s *DNSTestSuite) Test_TriageAndResolve_ConsulDomain() {
	mockedDNSConsulClient := mocks.NewDNSServiceClient(s.T())
	server := DNSServer{
		client:    mockedDNSConsulClient,
		logger:    hclog.Default(),
		partition: "test-partition",
		namespace: "test-namespace",
		token:     "test-token",
	}

	query := buildDNSQuery(s.T(), "service.default.consul")
	consulResp := buildDNSAnswerResponse(s.T(), "service.default.consul", "service.default.consul", dnsmessage.RCodeSuccess)

	mockedDNSConsulClient.On("Query", mock.Anything, mock.Anything).
		Return(&pbdns.QueryResponse{Msg: consulResp}, nil).
		Once()

	resp, err := server.triageAndResolve(query, pbdns.Protocol_PROTOCOL_UDP)
	s.Require().NoError(err)
	s.Require().Equal(consulResp, resp)
}

func (s *DNSTestSuite) Test_TriageAndResolve_ExternalDomain_EgressForwardingAndFallback() {
	s.Run("uses egress listener for external domains", func() {
		mockedDNSConsulClient := mocks.NewDNSServiceClient(s.T())
		query := buildDNSQuery(s.T(), "www.example.com")
		expectedResp := buildDNSAnswerResponse(s.T(), "www.example.com", "www.example.com", dnsmessage.RCodeSuccess)

		udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
		s.Require().NoError(err)
		defer udpConn.Close()

		done := make(chan struct{})
		go func() {
			defer close(done)
			buf := make([]byte, 4096)
			n, addr, readErr := udpConn.ReadFromUDP(buf)
			if readErr != nil {
				return
			}
			_, _ = udpConn.WriteToUDP(expectedResp, addr)
			_ = n
		}()

		server := DNSServer{
			client:               mockedDNSConsulClient,
			logger:               hclog.Default(),
			datacenter:           "dc1",
			virtualDNSEgressAddr: udpConn.LocalAddr().String(),
		}

		resp, err := server.triageAndResolve(query, pbdns.Protocol_PROTOCOL_UDP)
		s.Require().NoError(err)
		s.Require().Equal(expectedResp, resp)
		<-done
	})

	s.Run("falls back to consul on egress listener error", func() {
		mockedDNSConsulClient := mocks.NewDNSServiceClient(s.T())
		query := buildDNSQuery(s.T(), "www.example.com")
		consulResp := buildDNSAnswerResponse(s.T(), "www.example.com", "www.example.com", dnsmessage.RCodeSuccess)

		server := DNSServer{
			client:               mockedDNSConsulClient,
			logger:               hclog.Default(),
			partition:            "test-partition",
			namespace:            "test-namespace",
			token:                "test-token",
			datacenter:           "dc1",
			virtualDNSEgressAddr: "127.0.0.1:1",
		}

		mockedDNSConsulClient.On("Query", mock.Anything, mock.Anything).
			Return(&pbdns.QueryResponse{Msg: consulResp}, nil).
			Once()

		resp, err := server.triageAndResolve(query, pbdns.Protocol_PROTOCOL_UDP)
		s.Require().NoError(err)
		s.Require().Equal(consulResp, resp)
		s.Require().False(server.canTryEgressListener())
	})
}

func (s *DNSTestSuite) Test_TriageAndResolve_VirtualDomain() {
	s.Run("inline hit rewrites response back to original name", func() {
		mockedDNSConsulClient := mocks.NewDNSServiceClient(s.T())

		originalName := "service.virtual.consul"
		expandedName := "service.virtual.default.ns.partition-vms.ap.dc1.dc.consul"
		query := buildDNSQuery(s.T(), originalName)

		udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
		s.Require().NoError(err)
		defer udpConn.Close()

		done := make(chan struct{})
		go func() {
			defer close(done)
			buf := make([]byte, 4096)
			n, addr, readErr := udpConn.ReadFromUDP(buf)
			if readErr != nil {
				return
			}
			receivedName := firstQuestionNameFromRaw(s.T(), buf[:n])
			s.Equal(canonicalName(expandedName), receivedName)
			response := buildDNSAnswerResponse(s.T(), expandedName, expandedName, dnsmessage.RCodeSuccess)
			_, _ = udpConn.WriteToUDP(response, addr)
		}()

		server := DNSServer{
			client:               mockedDNSConsulClient,
			logger:               hclog.Default(),
			namespace:            "default",
			partition:            "partition-vms",
			datacenter:           "dc1",
			virtualDNSInlineAddr: udpConn.LocalAddr().String(),
		}

		resp, err := server.triageAndResolve(query, pbdns.Protocol_PROTOCOL_UDP)
		s.Require().NoError(err)
		s.Require().Equal(canonicalName(originalName), firstQuestionNameFromRaw(s.T(), resp))
		s.Require().Equal(canonicalName(originalName), firstAnswerNameFromRaw(s.T(), resp))
		<-done
	})

	s.Run("service alias inline hit rewrites response back to original name", func() {
		mockedDNSConsulClient := mocks.NewDNSServiceClient(s.T())

		originalName := "service-db.service.virtual.consul"
		expandedName := "service-db.virtual.default.ns.partition-vms.ap.dc1.dc.consul"
		query := buildDNSQuery(s.T(), originalName)

		udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
		s.Require().NoError(err)
		defer udpConn.Close()

		done := make(chan struct{})
		go func() {
			defer close(done)
			buf := make([]byte, 4096)
			n, addr, readErr := udpConn.ReadFromUDP(buf)
			if readErr != nil {
				return
			}
			receivedName := firstQuestionNameFromRaw(s.T(), buf[:n])
			s.Equal(canonicalName(expandedName), receivedName)
			response := buildDNSAnswerResponse(s.T(), expandedName, expandedName, dnsmessage.RCodeSuccess)
			_, _ = udpConn.WriteToUDP(response, addr)
		}()

		server := DNSServer{
			client:               mockedDNSConsulClient,
			logger:               hclog.Default(),
			namespace:            "default",
			partition:            "partition-vms",
			datacenter:           "dc1",
			virtualDNSInlineAddr: udpConn.LocalAddr().String(),
		}

		resp, err := server.triageAndResolve(query, pbdns.Protocol_PROTOCOL_UDP)
		s.Require().NoError(err)
		s.Require().Equal(canonicalName(originalName), firstQuestionNameFromRaw(s.T(), resp))
		s.Require().Equal(canonicalName(originalName), firstAnswerNameFromRaw(s.T(), resp))
		<-done
	})

	s.Run("nxdomain from inline listener falls back to consul", func() {
		mockedDNSConsulClient := mocks.NewDNSServiceClient(s.T())

		originalName := "service.virtual.consul"
		expandedName := "service.virtual.default.ns.partition-vms.ap.dc1.dc.consul"
		query := buildDNSQuery(s.T(), originalName)
		consulResp := buildDNSAnswerResponse(s.T(), originalName, originalName, dnsmessage.RCodeSuccess)

		udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
		s.Require().NoError(err)
		defer udpConn.Close()

		done := make(chan struct{})
		go func() {
			defer close(done)
			buf := make([]byte, 4096)
			n, addr, readErr := udpConn.ReadFromUDP(buf)
			if readErr != nil {
				return
			}
			receivedName := firstQuestionNameFromRaw(s.T(), buf[:n])
			s.Equal(canonicalName(expandedName), receivedName)
			nx := buildDNSRCodeResponse(s.T(), expandedName, dnsmessage.RCodeNameError)
			_, _ = udpConn.WriteToUDP(nx, addr)
		}()

		server := DNSServer{
			client:               mockedDNSConsulClient,
			logger:               hclog.Default(),
			partition:            "partition-vms",
			namespace:            "default",
			token:                "test-token",
			datacenter:           "dc1",
			virtualDNSInlineAddr: udpConn.LocalAddr().String(),
		}

		mockedDNSConsulClient.On("Query", mock.Anything, mock.Anything).
			Return(&pbdns.QueryResponse{Msg: consulResp}, nil).
			Once()

		resp, err := server.triageAndResolve(query, pbdns.Protocol_PROTOCOL_UDP)
		s.Require().NoError(err)
		s.Require().Equal(consulResp, resp)
		<-done
	})

	s.Run("non-success rcode from inline listener falls back to consul", func() {
		// Envoy returning FORMERR (or any non-RCodeSuccess rcode) must not be
		// treated as a successful hit; the request should fall through to Consul.
		mockedDNSConsulClient := mocks.NewDNSServiceClient(s.T())

		originalName := "service.virtual.consul"
		expandedName := "service.virtual.default.ns.partition-vms.ap.dc1.dc.consul"
		query := buildDNSQuery(s.T(), originalName)
		consulResp := buildDNSAnswerResponse(s.T(), originalName, originalName, dnsmessage.RCodeSuccess)

		udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
		s.Require().NoError(err)
		defer udpConn.Close()

		done := make(chan struct{})
		go func() {
			defer close(done)
			buf := make([]byte, 4096)
			n, addr, readErr := udpConn.ReadFromUDP(buf)
			if readErr != nil {
				return
			}
			receivedName := firstQuestionNameFromRaw(s.T(), buf[:n])
			s.Equal(canonicalName(expandedName), receivedName)
			// Envoy signals a malformed request — should NOT be returned to client.
			formerr := buildDNSRCodeResponse(s.T(), expandedName, dnsmessage.RCodeFormatError)
			_, _ = udpConn.WriteToUDP(formerr, addr)
		}()

		server := DNSServer{
			client:               mockedDNSConsulClient,
			logger:               hclog.Default(),
			partition:            "partition-vms",
			namespace:            "default",
			token:                "test-token",
			datacenter:           "dc1",
			virtualDNSInlineAddr: udpConn.LocalAddr().String(),
		}

		mockedDNSConsulClient.On("Query", mock.Anything, mock.Anything).
			Return(&pbdns.QueryResponse{Msg: consulResp}, nil).
			Once()

		resp, err := server.triageAndResolve(query, pbdns.Protocol_PROTOCOL_UDP)
		s.Require().NoError(err)
		s.Require().Equal(consulResp, resp)
		<-done
	})

	s.Run("inline listener error falls back to consul and marks listener unavailable", func() {
		mockedDNSConsulClient := mocks.NewDNSServiceClient(s.T())
		query := buildDNSQuery(s.T(), "service.virtual.consul")
		consulResp := buildDNSAnswerResponse(s.T(), "service.virtual.consul", "service.virtual.consul", dnsmessage.RCodeSuccess)

		server := DNSServer{
			client:               mockedDNSConsulClient,
			logger:               hclog.Default(),
			partition:            "partition-vms",
			namespace:            "default",
			token:                "test-token",
			datacenter:           "dc1",
			virtualDNSInlineAddr: "127.0.0.1:1",
		}

		mockedDNSConsulClient.On("Query", mock.Anything, mock.Anything).
			Return(&pbdns.QueryResponse{Msg: consulResp}, nil).
			Once()

		resp, err := server.triageAndResolve(query, pbdns.Protocol_PROTOCOL_UDP)
		s.Require().NoError(err)
		s.Require().Equal(consulResp, resp)
		s.Require().False(server.canTryInlineListener())
	})
}

func buildDNSQuery(t *testing.T, name string) []byte {
	t.Helper()
	qName, err := dnsmessage.NewName(canonicalName(name))
	require.NoError(t, err)

	msg := dnsmessage.Message{
		Header: dnsmessage.Header{ID: 1, RecursionDesired: true},
		Questions: []dnsmessage.Question{{
			Name:  qName,
			Type:  dnsmessage.TypeA,
			Class: dnsmessage.ClassINET,
		}},
	}

	raw, err := msg.Pack()
	require.NoError(t, err)
	return raw
}

func buildDNSRCodeResponse(t *testing.T, questionName string, rcode dnsmessage.RCode) []byte {
	t.Helper()
	qName, err := dnsmessage.NewName(canonicalName(questionName))
	require.NoError(t, err)

	msg := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:       1,
			Response: true,
			RCode:    rcode,
		},
		Questions: []dnsmessage.Question{{
			Name:  qName,
			Type:  dnsmessage.TypeA,
			Class: dnsmessage.ClassINET,
		}},
	}

	raw, err := msg.Pack()
	require.NoError(t, err)
	return raw
}

func buildDNSAnswerResponse(t *testing.T, questionName, answerName string, rcode dnsmessage.RCode) []byte {
	t.Helper()
	qName, err := dnsmessage.NewName(canonicalName(questionName))
	require.NoError(t, err)
	aName, err := dnsmessage.NewName(canonicalName(answerName))
	require.NoError(t, err)

	msg := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:       1,
			Response: true,
			RCode:    rcode,
		},
		Questions: []dnsmessage.Question{{
			Name:  qName,
			Type:  dnsmessage.TypeA,
			Class: dnsmessage.ClassINET,
		}},
		Answers: []dnsmessage.Resource{{
			Header: dnsmessage.ResourceHeader{
				Name:  aName,
				Type:  dnsmessage.TypeA,
				Class: dnsmessage.ClassINET,
				TTL:   1,
			},
			Body: &dnsmessage.AResource{A: [4]byte{127, 0, 0, 1}},
		}},
	}

	raw, err := msg.Pack()
	require.NoError(t, err)
	return raw
}

func firstQuestionNameFromRaw(t *testing.T, raw []byte) string {
	t.Helper()
	var msg dnsmessage.Message
	require.NoError(t, msg.Unpack(raw))
	require.NotEmpty(t, msg.Questions)
	return msg.Questions[0].Name.String()
}

func firstAnswerNameFromRaw(t *testing.T, raw []byte) string {
	t.Helper()
	var msg dnsmessage.Message
	require.NoError(t, msg.Unpack(raw))
	require.NotEmpty(t, msg.Answers)
	return msg.Answers[0].Header.Name.String()
}

// TestParseVirtualTokens verifies that all 8 alias forms produced by virtual
// FQDN queries are parsed correctly. The RFC mandates that the dataplane handles
// every combination of omitted ns/partition/dc qualifiers.
func TestParseVirtualTokens(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantSvc   string
		wantNS    string
		wantAP    string
		wantDC    string
		wantOK    bool
	}{
		// (1) bare — no qualifiers at all
		{
			name:    "bare no tokens",
			input:   "svc.virtual.consul",
			wantSvc: "svc",
			wantOK:  true,
		},
		// (2) .service. alias — stripped to base service name
		{
			name:    "service alias no tokens",
			input:   "svc.service.virtual.consul",
			wantSvc: "svc",
			wantOK:  true,
		},
		// (3) namespace only
		{
			name:    "namespace only",
			input:   "svc.virtual.myns.ns.consul",
			wantSvc: "svc",
			wantNS:  "myns",
			wantOK:  true,
		},
		// (4) partition only
		{
			name:    "partition only",
			input:   "svc.virtual.myap.ap.consul",
			wantSvc: "svc",
			wantAP:  "myap",
			wantOK:  true,
		},
		// (5) datacenter only
		{
			name:    "datacenter only",
			input:   "svc.virtual.dc2.dc.consul",
			wantSvc: "svc",
			wantDC:  "dc2",
			wantOK:  true,
		},
		// (6) namespace + partition, no dc
		{
			name:    "namespace and partition no dc",
			input:   "svc.virtual.myns.ns.myap.ap.consul",
			wantSvc: "svc",
			wantNS:  "myns",
			wantAP:  "myap",
			wantOK:  true,
		},
		// (7) namespace + dc, no partition
		{
			name:    "namespace and dc no partition",
			input:   "svc.virtual.myns.ns.dc2.dc.consul",
			wantSvc: "svc",
			wantNS:  "myns",
			wantDC:  "dc2",
			wantOK:  true,
		},
		// (8) partition + dc, no namespace
		{
			name:    "partition and dc no namespace",
			input:   "svc.virtual.myap.ap.dc2.dc.consul",
			wantSvc: "svc",
			wantAP:  "myap",
			wantDC:  "dc2",
			wantOK:  true,
		},
		// trailing-dot variant is tolerated
		{
			name:    "bare with trailing dot",
			input:   "svc.virtual.consul.",
			wantSvc: "svc",
			wantOK:  true,
		},
		// not a virtual domain
		{
			name:   "non-virtual consul domain",
			input:  "svc.service.default.consul",
			wantOK: false,
		},

		// ── named-port virtual DNS form (<port>.<svc>.virtual.consul) ────────
		// Consul DNS parses "http.api.virtual.consul" as port "http", service
		// "api" (agent/dns.go: queryParts = ["http", "api"]). The Envoy inline
		// DNS table is keyed on the base service name only, so the port label
		// must be stripped and the returned svc must be "api".
		{
			name:    "named-port bare no qualifiers",
			input:   "http.api.virtual.consul",
			wantSvc: "api",
			wantOK:  true,
		},
		{
			name:    "named-port with namespace only",
			input:   "http.api.virtual.myns.ns.consul",
			wantSvc: "api",
			wantNS:  "myns",
			wantOK:  true,
		},
		{
			name:    "named-port with partition only",
			input:   "http.api.virtual.myap.ap.consul",
			wantSvc: "api",
			wantAP:  "myap",
			wantOK:  true,
		},
		{
			name:    "named-port with datacenter only",
			input:   "http.api.virtual.dc2.dc.consul",
			wantSvc: "api",
			wantDC:  "dc2",
			wantOK:  true,
		},
		{
			name:    "named-port with all qualifiers",
			input:   "http.api.virtual.myns.ns.myap.ap.dc1.dc.consul",
			wantSvc: "api",
			wantNS:  "myns",
			wantAP:  "myap",
			wantDC:  "dc1",
			wantOK:  true,
		},
		// .service alias combined with named-port prefix
		{
			name:    "named-port with service alias no qualifiers",
			input:   "http.api.service.virtual.consul",
			wantSvc: "api",
			wantOK:  true,
		},
		{
			name:    "named-port with service alias and namespace",
			input:   "http.api.service.virtual.myns.ns.consul",
			wantSvc: "api",
			wantNS:  "myns",
			wantOK:  true,
		},
		// trailing-dot variant with named port
		{
			name:    "named-port with trailing dot",
			input:   "http.api.virtual.consul.",
			wantSvc: "api",
			wantOK:  true,
		},
		// Three labels before .virtual. is not a recognised form and must be
		// rejected (would be <a>.<b>.<c>.virtual.* — no Consul query produces this).
		{
			name:   "three prefix labels before virtual rejected",
			input:  "a.b.c.virtual.consul",
			wantOK: false,
		},

		// ── collision regression (GitHub issue) ──────────────────────────────
		// A Connect-native service literally named "virtual" registered with tag
		// "blue" produces the standard Consul tagged-service query:
		//   <tag>.<service>.service.consul  →  blue.virtual.service.consul
		// Consul interprets "service" as the query-kind label, not a virtual
		// qualifier, so this must be classified as domainClassConsul and routed
		// to the Consul server unchanged. Previously the substring check
		// strings.Contains(name, ".virtual.") caused a false match here.
		{
			name:   "service-kind label after virtual is not a virtual domain",
			input:  "blue.virtual.service.consul",
			wantOK: false,
		},
		// Same collision via the ".node." query-kind.
		{
			name:   "node-kind label after virtual is not a virtual domain",
			input:  "virtual.node.dc1.consul",
			wantOK: false,
		},
		// Odd number of remainder labels (not pairable) must be rejected.
		{
			name:   "odd remainder labels rejected",
			input:  "svc.virtual.myns.consul",
			wantOK: false,
		},
		// An unrecognised qualifier must not silently be swallowed.
		{
			name:   "unrecognised qualifier rejected",
			input:  "svc.virtual.myns.tag.consul",
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotSvc, gotNS, gotAP, gotDC, gotOK := parseVirtualTokens(tc.input)
			if gotOK != tc.wantOK {
				t.Fatalf("parseVirtualTokens(%q) ok = %v, want %v", tc.input, gotOK, tc.wantOK)
			}
			if !gotOK {
				return
			}
			if gotSvc != tc.wantSvc {
				t.Errorf("svc: got %q, want %q", gotSvc, tc.wantSvc)
			}
			if gotNS != tc.wantNS {
				t.Errorf("ns: got %q, want %q", gotNS, tc.wantNS)
			}
			if gotAP != tc.wantAP {
				t.Errorf("partition: got %q, want %q", gotAP, tc.wantAP)
			}
			if gotDC != tc.wantDC {
				t.Errorf("dc: got %q, want %q", gotDC, tc.wantDC)
			}
		})
	}
}

// TestExpandVirtualName verifies that expandVirtualName produces the canonical
// <svc>.virtual.<ns>.ns.<ap>.ap.<dc>.dc.consul FQDN for every alias form.
// The upstream index is used where it can provide a unique match; the server
// defaults fill any remaining gaps, exactly as specified by the RFC.
func TestExpandVirtualName(t *testing.T) {
	const td = "e5b1a4d3.consul"

	// Index contains one service with a fully-qualified identity and a second
	// service that appears in two datacenters so that disambiguation tests work.
	idx := NewUpstreamIndex()
	idx.Update([]string{
		"api.myns.myap.dc1.internal-v1." + td,          // unique: api in myns/myap/dc1
		"shared.default.default.dc1.internal-v1." + td,  // shared in dc1
		"shared.default.default.dc2.internal-v1." + td,  // shared in dc2 — ambiguous without dc
	}, nil)

	// server defaults used when neither the query nor the index fills a token.
	srv := &DNSServer{
		namespace:     "default",
		partition:     "default",
		datacenter:    "dc1",
		upstreamIndex: idx,
	}

	cases := []struct {
		name  string
		input string
		want  string
	}{
		// (1) bare — index fills ns/ap/dc from unique lookup
		{
			name:  "bare resolved from index",
			input: "api.virtual.consul",
			want:  "api.virtual.myns.ns.myap.ap.dc1.dc.consul",
		},
		// (2) .service. alias — same expansion as bare
		{
			name:  "service alias resolved from index",
			input: "api.service.virtual.consul",
			want:  "api.virtual.myns.ns.myap.ap.dc1.dc.consul",
		},
		// (3) namespace only — index lookup constrained by ns
		{
			name:  "namespace only resolved from index",
			input: "api.virtual.myns.ns.consul",
			want:  "api.virtual.myns.ns.myap.ap.dc1.dc.consul",
		},
		// (4) partition only
		{
			name:  "partition only resolved from index",
			input: "api.virtual.myap.ap.consul",
			want:  "api.virtual.myns.ns.myap.ap.dc1.dc.consul",
		},
		// (5) datacenter only
		{
			name:  "datacenter only resolved from index",
			input: "api.virtual.dc1.dc.consul",
			want:  "api.virtual.myns.ns.myap.ap.dc1.dc.consul",
		},
		// (6) namespace + partition — index fills dc
		{
			name:  "namespace and partition resolved from index",
			input: "api.virtual.myns.ns.myap.ap.consul",
			want:  "api.virtual.myns.ns.myap.ap.dc1.dc.consul",
		},
		// (7) namespace + dc — index fills partition
		{
			name:  "namespace and dc resolved from index",
			input: "api.virtual.myns.ns.dc1.dc.consul",
			want:  "api.virtual.myns.ns.myap.ap.dc1.dc.consul",
		},
		// (8) partition + dc — index fills namespace
		{
			name:  "partition and dc resolved from index",
			input: "api.virtual.myap.ap.dc1.dc.consul",
			want:  "api.virtual.myns.ns.myap.ap.dc1.dc.consul",
		},
		// index lookup is ambiguous (two DCs); server defaults fill the gaps
		{
			name:  "ambiguous lookup falls back to server defaults",
			input: "shared.virtual.consul",
			want:  "shared.virtual.default.ns.default.ap.dc1.dc.consul",
		},
		// dc qualifier disambiguates what would otherwise be ambiguous
		{
			name:  "dc qualifier disambiguates shared service",
			input: "shared.virtual.dc2.dc.consul",
			want:  "shared.virtual.default.ns.default.ap.dc2.dc.consul",
		},
		// unknown service — no index hit; server defaults fill all tokens
		{
			name:  "unknown service falls back to server defaults",
			input: "unknown.virtual.consul",
			want:  "unknown.virtual.default.ns.default.ap.dc1.dc.consul",
		},
		// fully-qualified input passes through unchanged (already canonical)
		{
			name:  "fully qualified passthrough",
			input: "api.virtual.myns.ns.myap.ap.dc1.dc.consul",
			want:  "api.virtual.myns.ns.myap.ap.dc1.dc.consul",
		},

		// ── named-port form: port label is stripped, base name drives lookup ─
		// "http.api.virtual.consul" must expand identically to "api.virtual.consul"
		// because the Envoy inline DNS table is keyed on the base service name.
		{
			name:  "named-port bare expands same as base service",
			input: "http.api.virtual.consul",
			want:  "api.virtual.myns.ns.myap.ap.dc1.dc.consul",
		},
		{
			name:  "named-port with all qualifiers expands same as base service",
			input: "http.api.virtual.myns.ns.myap.ap.dc1.dc.consul",
			want:  "api.virtual.myns.ns.myap.ap.dc1.dc.consul",
		},
		{
			name:  "named-port with service alias expands same as base service",
			input: "http.api.service.virtual.consul",
			want:  "api.virtual.myns.ns.myap.ap.dc1.dc.consul",
		},
		// Named port on an ambiguous service still falls back to server defaults.
		{
			name:  "named-port ambiguous falls back to server defaults",
			input: "http.shared.virtual.consul",
			want:  "shared.virtual.default.ns.default.ap.dc1.dc.consul",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := srv.expandVirtualName(tc.input)
			if got != tc.want {
				t.Errorf("expandVirtualName(%q)\n got  %q\n want %q", tc.input, got, tc.want)
			}
		})
	}

	// ── multiport end-to-end scenario ─────────────────────────────────────────
	// Simulate the full pipeline:
	//   1. A CDS delta arrives carrying two named-port cluster SNIs for the
	//      same multiport service (one per port).  ParseServiceSNI strips the
	//      port label and indexes both under the base service name "multi".
	//   2. A named-port virtual DNS query "http.multi.virtual.consul" arrives.
	//      expandVirtualName strips "http", calls Lookup("multi", …), and must
	//      produce the canonical FQDN using the identity from the index.
	t.Run("multiport end-to-end", func(t *testing.T) {
		mpIdx := NewUpstreamIndex()
		mpIdx.Update([]string{
			"http.multi.myns.myap.dc1.internal-v1." + td,
			"grpc.multi.myns.myap.dc1.internal-v1." + td,
		}, nil)
		mpSrv := &DNSServer{
			namespace:     "default",
			partition:     "default",
			datacenter:    "default-dc",
			upstreamIndex: mpIdx,
		}
		cases := []struct {
			input string
			want  string
		}{
			// port "http" stripped → base "multi" → index hit fills ns/ap/dc
			{"http.multi.virtual.consul", "multi.virtual.myns.ns.myap.ap.dc1.dc.consul"},
			// port "grpc" stripped → same base identity
			{"grpc.multi.virtual.consul", "multi.virtual.myns.ns.myap.ap.dc1.dc.consul"},
			// explicit qualifiers with named port
			{"http.multi.virtual.myns.ns.myap.ap.dc1.dc.consul", "multi.virtual.myns.ns.myap.ap.dc1.dc.consul"},
			// base-service form still works alongside the named-port form
			{"multi.virtual.consul", "multi.virtual.myns.ns.myap.ap.dc1.dc.consul"},
		}
		for _, tc := range cases {
			got := mpSrv.expandVirtualName(tc.input)
			if got != tc.want {
				t.Errorf("expandVirtualName(%q)\n got  %q\n want %q", tc.input, got, tc.want)
			}
		}
	})
}
