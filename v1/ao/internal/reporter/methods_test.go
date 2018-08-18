// Copyright (C) 2018 Librato, Inc. All rights reserved.

package reporter

import (
	"context"
	"testing"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter/collector"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestPostEventsMethod(t *testing.T) {
	pe := newPostEventsMethod(
		"test-ket",
		[][]byte{
			[]byte("hello"),
			[]byte("world"),
		})
	assert.Equal(t, "PostEvents", pe.String())
	assert.Equal(t, true, pe.RetryOnErr())
	assert.EqualValues(t, 2, pe.MessageLen())

	result := &collector.MessageResult{}
	mockTC := &mocks.TraceCollectorClient{}
	mockTC.On("PostEvents", mock.Anything, mock.Anything).
		Return(result, nil)

	err := pe.Call(context.Background(), mockTC)
	assert.Nil(t, err)
	assert.Equal(t, collector.ResultCode_OK, pe.ResultCode())
	assert.Equal(t, "", pe.Arg())
}

func TestPostMetricsMethod(t *testing.T) {
	pe := newPostMetricsMethod(
		"test-ket",
		[][]byte{
			[]byte("hello"),
			[]byte("world"),
		})
	assert.Equal(t, "PostMetrics", pe.String())
	assert.Equal(t, true, pe.RetryOnErr())
	assert.EqualValues(t, 2, pe.MessageLen())

	result := &collector.MessageResult{}
	mockTC := &mocks.TraceCollectorClient{}
	mockTC.On("PostMetrics", mock.Anything, mock.Anything).
		Return(result, nil)

	err := pe.Call(context.Background(), mockTC)
	assert.Nil(t, err)
	assert.Equal(t, collector.ResultCode_OK, pe.ResultCode())
	assert.Equal(t, "", pe.Arg())
}

func TestPostStatusMethod(t *testing.T) {
	pe := newPostStatusMethod(
		"test-ket",
		[][]byte{
			[]byte("hello"),
			[]byte("world"),
		})
	assert.Equal(t, "PostStatus", pe.String())
	assert.Equal(t, true, pe.RetryOnErr())
	assert.EqualValues(t, 2, pe.MessageLen())

	result := &collector.MessageResult{}
	mockTC := &mocks.TraceCollectorClient{}
	mockTC.On("PostStatus", mock.Anything, mock.Anything).
		Return(result, nil)

	err := pe.Call(context.Background(), mockTC)
	assert.Nil(t, err)
	assert.Equal(t, collector.ResultCode_OK, pe.ResultCode())
	assert.Equal(t, "", pe.Arg())
}

func TestGetSettingsMethod(t *testing.T) {
	pe := newGetSettingsMethod("test-ket")
	assert.Equal(t, "GetSettings", pe.String())
	assert.Equal(t, true, pe.RetryOnErr())
	assert.EqualValues(t, 0, pe.MessageLen())

	result := &collector.SettingsResult{}
	mockTC := &mocks.TraceCollectorClient{}
	mockTC.On("GetSettings", mock.Anything, mock.Anything).
		Return(result, nil)

	err := pe.Call(context.Background(), mockTC)
	assert.Nil(t, err)
	assert.Equal(t, collector.ResultCode_OK, pe.ResultCode())
	assert.Equal(t, "", pe.Arg())
}

func TestPingMethod(t *testing.T) {
	pe := newPingMethod("test-ket", "testConn")
	assert.Equal(t, "Ping testConn", pe.String())
	assert.Equal(t, false, pe.RetryOnErr())
	assert.EqualValues(t, 0, pe.MessageLen())

	result := &collector.MessageResult{}
	mockTC := &mocks.TraceCollectorClient{}
	mockTC.On("Ping", mock.Anything, mock.Anything).
		Return(result, nil)

	err := pe.Call(context.Background(), mockTC)
	assert.Nil(t, err)
	assert.Equal(t, collector.ResultCode_OK, pe.ResultCode())
	assert.Equal(t, "", pe.Arg())
}
