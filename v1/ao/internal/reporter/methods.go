// Copyright (C) 2018 Librato, Inc. All rights reserved.

package reporter

import (
	"context"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter/collector"
)

// Method defines the interface of an RPC call
type Method interface {
	// Call invokes the RPC method through a specified gRPC client
	Call(ctx context.Context, c collector.TraceCollectorClient) error

	// MessageLen returns the length (number of elements) of the method request
	MessageLen() int64

	// ResultCode is the return code of the RPC call. It must only be called after
	// the Call() method is invoked.
	ResultCode() collector.ResultCode

	// Arg returns the Arg field of the RPC call result. It must only be called
	// after the Call() method is invoked.
	Arg() string

	// String returns the string representation of the Method object
	String() string

	// RetryOnErr checks if the method allows retry
	RetryOnErr() bool
}

// PostEventsMethod is the struct for RPC method PostEvents
type PostEventsMethod struct {
	serviceKey string
	messages   [][]byte
	Resp       *collector.MessageResult
}

func newPostEventsMethod(key string, msgs [][]byte) *PostEventsMethod {
	return &PostEventsMethod{
		serviceKey: key,
		messages:   msgs,
	}
}

func (pe *PostEventsMethod) String() string {
	return "PostEvents"
}

func (pe *PostEventsMethod) ResultCode() collector.ResultCode {
	return pe.Resp.Result
}

func (pe *PostEventsMethod) Arg() string {
	return pe.Resp.Arg
}

func (pe *PostEventsMethod) MessageLen() int64 {
	return int64(len(pe.messages))
}

func (pe *PostEventsMethod) Call(ctx context.Context,
	c collector.TraceCollectorClient) error {
	request := &collector.MessageRequest{
		ApiKey:   pe.serviceKey,
		Messages: pe.messages,
		Encoding: collector.EncodingType_BSON,
		Identity: buildIdentity(),
	}
	var err error
	pe.Resp, err = c.PostEvents(ctx, request)
	return err
}

func (pe *PostEventsMethod) RetryOnErr() bool {
	return true
}

// PostMetricsMethod is the struct for RPC method PostMetrics
type PostMetricsMethod struct {
	serviceKey string
	messages   [][]byte
	Resp       *collector.MessageResult
}

func newPostMetricsMethod(key string, msgs [][]byte) *PostMetricsMethod {
	return &PostMetricsMethod{
		serviceKey: key,
		messages:   msgs,
	}
}

func (pm *PostMetricsMethod) String() string {
	return "PostMetrics"
}

func (pm *PostMetricsMethod) ResultCode() collector.ResultCode {
	return pm.Resp.Result
}

func (pm *PostMetricsMethod) Arg() string {
	return pm.Resp.Arg
}

func (pm *PostMetricsMethod) MessageLen() int64 {
	return int64(len(pm.messages))
}

func (pm *PostMetricsMethod) Call(ctx context.Context,
	c collector.TraceCollectorClient) error {
	request := &collector.MessageRequest{
		ApiKey:   pm.serviceKey,
		Messages: pm.messages,
		Encoding: collector.EncodingType_BSON,
		Identity: buildIdentity(),
	}
	var err error
	pm.Resp, err = c.PostMetrics(ctx, request)
	return err
}

func (pm *PostMetricsMethod) RetryOnErr() bool {
	return true
}

// PostStatusMethod is the struct for RPC method PostStatus
type PostStatusMethod struct {
	serviceKey string
	messages   [][]byte
	Resp       *collector.MessageResult
}

func newPostStatusMethod(key string, msgs [][]byte) *PostStatusMethod {
	return &PostStatusMethod{
		serviceKey: key,
		messages:   msgs,
	}
}

func (ps *PostStatusMethod) String() string {
	return "PostStatus"
}

func (ps *PostStatusMethod) ResultCode() collector.ResultCode {
	return ps.Resp.Result
}

func (ps *PostStatusMethod) Arg() string {
	return ps.Resp.Arg
}

func (ps *PostStatusMethod) MessageLen() int64 {
	return int64(len(ps.messages))
}

func (ps *PostStatusMethod) Call(ctx context.Context,
	c collector.TraceCollectorClient) error {
	request := &collector.MessageRequest{
		ApiKey:   ps.serviceKey,
		Messages: ps.messages,
		Encoding: collector.EncodingType_BSON,
		Identity: buildIdentity(),
	}
	var err error
	ps.Resp, err = c.PostStatus(ctx, request)
	return err
}

func (ps *PostStatusMethod) RetryOnErr() bool {
	return true
}

// GetSettingsMethod is the struct for RPC method GetSettings
type GetSettingsMethod struct {
	serviceKey string
	Resp       *collector.SettingsResult
}

func newGetSettingsMethod(key string) *GetSettingsMethod {
	return &GetSettingsMethod{
		serviceKey: key,
	}
}

func (gs *GetSettingsMethod) String() string {
	return "GetSettings"
}

func (gs *GetSettingsMethod) ResultCode() collector.ResultCode {
	return gs.Resp.Result
}

func (gs *GetSettingsMethod) Arg() string {
	return gs.Resp.Arg
}

func (ps *GetSettingsMethod) MessageLen() int64 {
	return 0
}

func (gs *GetSettingsMethod) Call(ctx context.Context,
	c collector.TraceCollectorClient) error {
	request := &collector.SettingsRequest{
		ApiKey:        gs.serviceKey,
		ClientVersion: grpcReporterVersion,
		Identity:      buildBestEffortIdentity(),
	}
	var err error
	gs.Resp, err = c.GetSettings(ctx, request)
	return err
}

func (gs *GetSettingsMethod) RetryOnErr() bool {
	return true
}

type PingMethod struct {
	conn       string
	serviceKey string
	Resp       *collector.MessageResult
}

func newPingMethod(key string, conn string) *PingMethod {
	return &PingMethod{
		serviceKey: key,
		conn:       conn,
	}
}

func (p *PingMethod) String() string {
	return "Ping " + p.conn
}

func (p *PingMethod) ResultCode() collector.ResultCode {
	return p.Resp.Result
}

func (p *PingMethod) Arg() string {
	return p.Resp.Arg
}

func (p *PingMethod) MessageLen() int64 {
	return 0
}

func (p *PingMethod) Call(ctx context.Context,
	c collector.TraceCollectorClient) error {
	request := &collector.PingRequest{
		ApiKey: p.serviceKey,
	}
	var err error
	p.Resp, err = c.Ping(ctx, request)
	return err
}

func (p *PingMethod) RetryOnErr() bool {
	return false
}
