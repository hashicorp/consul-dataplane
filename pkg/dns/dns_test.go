// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package dns

import (
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

type MockedNetConn struct {
	net.Conn
	mock.Mock
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
		"service.virtual.default.ns.default.ap.dc1.dc.consul": domainClassVirtual,
		"service.default.consul":                               domainClassConsul,
		"consul":                                               domainClassConsul,
		"google.com":                                           domainClassExternal,
	}

	for domain, expected := range testCases {
		s.Run(domain, func() {
			s.Require().Equal(expected, classifyDomain(domain))
		})
	}
}

func (s *DNSTestSuite) Test_ExpandVirtualFQDN() {
	testCases := []struct {
		name      string
		input     string
		expected  string
		defaultNS string
		defaultAP string
		defaultDC string
	}{
		{
			name:      "short form",
			input:     "service.virtual.consul",
			expected:  "service.virtual.default.ns.default.ap.dc1.dc.consul",
			defaultNS: "default",
			defaultAP: "default",
			defaultDC: "dc1",
		},
		{
			name:      "override namespace and datacenter",
			input:     "service.virtual.other.ns.dc2.dc.consul",
			expected:  "service.virtual.other.ns.default.ap.dc2.dc.consul",
			defaultNS: "default",
			defaultAP: "default",
			defaultDC: "dc1",
		},
		{
			name:      "override all qualifiers",
			input:     "service.virtual.team.ns.part.ap.dc2.dc.consul",
			expected:  "service.virtual.team.ns.part.ap.dc2.dc.consul",
			defaultNS: "default",
			defaultAP: "default",
			defaultDC: "dc1",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			out := expandVirtualFQDN(tc.input, tc.defaultNS, tc.defaultAP, tc.defaultDC)
			s.Require().Equal(tc.expected, out)
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
