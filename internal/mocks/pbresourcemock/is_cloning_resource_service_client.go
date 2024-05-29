// Code generated by mockery v2.32.0. DO NOT EDIT.

package pbresourcemock

import mock "github.com/stretchr/testify/mock"

// IsCloningResourceServiceClient is an autogenerated mock type for the IsCloningResourceServiceClient type
type IsCloningResourceServiceClient struct {
	mock.Mock
}

// IsCloningResourceServiceClient provides a mock function with given fields:
func (_m *IsCloningResourceServiceClient) IsCloningResourceServiceClient() bool {
	ret := _m.Called()

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// NewIsCloningResourceServiceClient creates a new instance of IsCloningResourceServiceClient. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewIsCloningResourceServiceClient(t interface {
	mock.TestingT
	Cleanup(func())
}) *IsCloningResourceServiceClient {
	mock := &IsCloningResourceServiceClient{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}