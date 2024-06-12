// Code generated by mockery v2.32.0. DO NOT EDIT.

package pbdnsmock

import (
	context "context"

	grpc "google.golang.org/grpc"

	mock "github.com/stretchr/testify/mock"

	pbdns "github.com/hashicorp/consul/proto-public/pbdns"
)

// DNSServiceClient is an autogenerated mock type for the DNSServiceClient type
type DNSServiceClient struct {
	mock.Mock
}

// Query provides a mock function with given fields: ctx, in, opts
func (_m *DNSServiceClient) Query(ctx context.Context, in *pbdns.QueryRequest, opts ...grpc.CallOption) (*pbdns.QueryResponse, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *pbdns.QueryResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *pbdns.QueryRequest, ...grpc.CallOption) (*pbdns.QueryResponse, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *pbdns.QueryRequest, ...grpc.CallOption) *pbdns.QueryResponse); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*pbdns.QueryResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *pbdns.QueryRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewDNSServiceClient creates a new instance of DNSServiceClient. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewDNSServiceClient(t interface {
	mock.TestingT
	Cleanup(func())
}) *DNSServiceClient {
	mock := &DNSServiceClient{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}