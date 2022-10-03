package dns

import (
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
	"github.com/stretchr/testify/suite"
)

type MockedNetConn struct {
	net.Conn
	mock.Mock
}

type DNSTestSuite struct {
	suite.Suite
}

// func (s *DNSTestSuite) SetupTest() {

// }

// func (s *DNSTestSuite) AfterTest() {

// }

func TestDNS_suite(t *testing.T) {
	suite.Run(t, new(DNSTestSuite))
}

func genRandomBytes(size int) (blk []byte) {
	blk = make([]byte, size)
	_, _ = rand.Read(blk)
	return blk
}

func (s *DNSTestSuite) Test_DisabledServer() {
	mockedDNSConsulClient := pbdns.NewMockDNSServiceClient(s.T())
	server, err := NewDNSServer(DNSServerParams{
		BindAddr: "127.0.0.1",
		Port:     -1, // disabled server
		Logger:   hclog.Default(),
		Client:   mockedDNSConsulClient,
	})
	if err != nil {
		s.T().FailNow()
	}
	err = server.Run()
	s.Require().Equal(ErrServerDisabled, err)
	s.Require().Equal(server.TcpPort(), -1)
	s.Require().Equal(server.UdpPort(), -1)
	server.Stop()

}

func (s *DNSTestSuite) Test_ServerStop() {
	mockedDNSConsulClient := pbdns.NewMockDNSServiceClient(s.T())
	server, err := NewDNSServer(DNSServerParams{
		BindAddr: "127.0.0.1",
		Port:     0, // let the os choose a port
		Logger:   hclog.Default(),
		Client:   mockedDNSConsulClient,
	})
	if err != nil {
		s.T().FailNow()
	}
	err = server.Run()
	if err != nil {
		s.T().FailNow()
	}
	server.Stop()

	s.Require().Eventually(func() bool {
		port := server.TcpPort()
		addr := fmt.Sprintf("127.0.0.1:%v", port)
		_, err := net.Dial("tcp", addr)
		s.T().Logf("dial error: %v", err)
		return err != nil
	}, time.Second*5, time.Second, "Failure to shut down tcp")

	s.Require().Eventually(func() bool {
		port := server.TcpPort()
		addr := fmt.Sprintf("127.0.0.1:%v", port)
		c, _ := net.Dial("udp", addr)
		_, _ = c.Write([]byte("here"))
		p := make([]byte, 512)
		_, err = c.Read(p)
		s.T().Logf("read udp error: %v", err)
		return err != nil
	}, time.Second*5, time.Second, "Failure to shut down udp")
}

func (s *DNSTestSuite) Test_UDPProxy() {
	mockedDNSConsulClient := pbdns.NewMockDNSServiceClient(s.T())
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
	connUdp, err := net.ListenUDP("udp", addr)
	s.Require().NoError(err)
	stopChan := make(chan struct{})
	defer func() { stopChan <- struct{}{} }()

	server := DNSServer{
		client:  mockedDNSConsulClient,
		connUDP: connUdp,
		logger:  hclog.Default(),
		stopCh:  stopChan,
	}

	go server.proxyUDP()

	testCases := map[string]struct {
		dnsRequest   []byte
		dnsResp      []byte
		expected     error
		largeResp    error
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
	mockedDNSConsulClient := pbdns.NewMockDNSServiceClient(s.T())
	addr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
	listenerTCP, err := net.ListenTCP("tcp", addr)
	s.Require().NoError(err)

	stopChan := make(chan struct{})
	defer close(stopChan)
	server := DNSServer{
		client:      mockedDNSConsulClient,
		listenerTCP: listenerTCP,
		logger:      hclog.Default(),
		stopCh:      stopChan,
	}

	go server.proxyTCP()

	testCases := map[string]struct {
		dnsRequest   []byte
		dnsResp      []byte
		expected     error
		largeResp    error
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
			largeResp:  errors.New("EOF"),
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
			if tc.largeResp != nil || tc.expectedGRPC != nil {
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
