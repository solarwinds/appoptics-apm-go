// Code generated by mockery v1.0.0. DO NOT EDIT.

package mocks

import (
	context "context"

	collector "github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter/collector"

	mock "github.com/stretchr/testify/mock"
)

// Method is an autogenerated mock type for the Method type
type Method struct {
	mock.Mock
}

// Arg provides a mock function with given fields:
func (_m *Method) Arg() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// Call provides a mock function with given fields: ctx, c
func (_m *Method) Call(ctx context.Context, c collector.TraceCollectorClient) error {
	ret := _m.Called(ctx, c)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, collector.TraceCollectorClient) error); ok {
		r0 = rf(ctx, c)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// CallSummary provides a mock function with given fields:
func (_m *Method) CallSummary() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// Message provides a mock function with given fields:
func (_m *Method) Message() [][]byte {
	ret := _m.Called()

	var r0 [][]byte
	if rf, ok := ret.Get(0).(func() [][]byte); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([][]byte)
		}
	}

	return r0
}

// MessageLen provides a mock function with given fields:
func (_m *Method) MessageLen() int64 {
	ret := _m.Called()

	var r0 int64
	if rf, ok := ret.Get(0).(func() int64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int64)
	}

	return r0
}

// RequestSize provides a mock function with given fields:
func (_m *Method) RequestSize() int64 {
	ret := _m.Called()

	var r0 int64
	if rf, ok := ret.Get(0).(func() int64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int64)
	}

	return r0
}

// ResultCode provides a mock function with given fields:
func (_m *Method) ResultCode() (collector.ResultCode, error) {
	ret := _m.Called()

	var r0 collector.ResultCode
	if rf, ok := ret.Get(0).(func() collector.ResultCode); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(collector.ResultCode)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// RetryOnErr provides a mock function with given fields: _a0
func (_m *Method) RetryOnErr(_a0 error) bool {
	ret := _m.Called(_a0)

	var r0 bool
	if rf, ok := ret.Get(0).(func(error) bool); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// ServiceKey provides a mock function with given fields:
func (_m *Method) ServiceKey() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// String provides a mock function with given fields:
func (_m *Method) String() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}
