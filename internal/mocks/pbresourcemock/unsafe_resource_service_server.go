// Code generated by mockery v2.32.0. DO NOT EDIT.

package pbresourcemock

import mock "github.com/stretchr/testify/mock"

// UnsafeResourceServiceServer is an autogenerated mock type for the UnsafeResourceServiceServer type
type UnsafeResourceServiceServer struct {
	mock.Mock
}

// mustEmbedUnimplementedResourceServiceServer provides a mock function with given fields:
func (_m *UnsafeResourceServiceServer) mustEmbedUnimplementedResourceServiceServer() {
	_m.Called()
}

// NewUnsafeResourceServiceServer creates a new instance of UnsafeResourceServiceServer. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewUnsafeResourceServiceServer(t interface {
	mock.TestingT
	Cleanup(func())
}) *UnsafeResourceServiceServer {
	mock := &UnsafeResourceServiceServer{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
