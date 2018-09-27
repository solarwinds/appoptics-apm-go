// Copyright (C) 2018 Librato, Inc. All rights reserved.

package reporter

import (
	"context"
	"fmt"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter/collector"
)

// Method defines the interface of an RPC call
type Method interface {
	// Call invokes the RPC method through a specified gRPC client
	Call(ctx context.Context, c collector.TraceCollectorClient) error

	// CallSummary returns a string indicating the summary of this call, e.g., the
	// data chunks sent out and the round trip time.It is for logging purpose only.
	CallSummary() string

	// MessageLen returns the length (number of elements) of the method request
	MessageLen() int64

	// Message returns the message body, if any
	Message() [][]byte

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
	rtt        time.Duration
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

// ResultCode returns the result code of the PostEvents RPC call. This method
// should only be invoked after the method Call is called.
func (pe *PostEventsMethod) ResultCode() collector.ResultCode {
	return pe.Resp.Result
}

// Arg returns the Arg of the PostEvents RPC call. This method
// should only be invoked after the method Call is called.
func (pe *PostEventsMethod) Arg() string {
	return pe.Resp.Arg
}

// MessageLen returns the length of the PostEvents RPC message body.
func (pe *PostEventsMethod) MessageLen() int64 {
	return int64(len(pe.messages))
}

// Message returns the message body of the RPC call
func (pe *PostEventsMethod) Message() [][]byte {
	return pe.messages
}

// Call issues an RPC call with the provided information
func (pe *PostEventsMethod) Call(ctx context.Context,
	c collector.TraceCollectorClient) error {
	request := &collector.MessageRequest{
		ApiKey:   pe.serviceKey,
		Messages: pe.messages,
		Encoding: collector.EncodingType_BSON,
		Identity: buildIdentity(),
	}
	var err error
	start := time.Now()
	pe.Resp, err = c.PostEvents(ctx, request)
	pe.rtt = time.Now().Sub(start)
	return err
}

// CallSummary returns a string representation of the RPC call result.
// It is mainly used for debug printing.
func (pe *PostEventsMethod) CallSummary() string {
	rsp := fmt.Sprintf("%v %s", pe.Resp.Result, pe.Resp.Arg)
	return fmt.Sprintf("[%s] sent %d events, rtt=%v. rsp=%s",
		pe, pe.MessageLen(), pe.rtt, rsp)
}

// RetryOnErr denotes if retry is needed for this RPC method
func (pe *PostEventsMethod) RetryOnErr() bool {
	return true
}

// PostMetricsMethod is the struct for RPC method PostMetrics
type PostMetricsMethod struct {
	serviceKey string
	messages   [][]byte
	Resp       *collector.MessageResult
	rtt        time.Duration
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

// ResultCode returns the result code of the RPC call. This method should
// only be called after the method Call is invoked.
func (pm *PostMetricsMethod) ResultCode() collector.ResultCode {
	return pm.Resp.Result
}

// Arg returns the Arg of the RPC call result
func (pm *PostMetricsMethod) Arg() string {
	return pm.Resp.Arg
}

// MessageLen returns the length of the RPC call message.
func (pm *PostMetricsMethod) MessageLen() int64 {
	return int64(len(pm.messages))
}

// Message returns the message body of the RPC call.
func (pm *PostMetricsMethod) Message() [][]byte {
	return pm.messages
}

// Call issues an RPC request with the provided information.
func (pm *PostMetricsMethod) Call(ctx context.Context,
	c collector.TraceCollectorClient) error {
	request := &collector.MessageRequest{
		ApiKey:   pm.serviceKey,
		Messages: pm.messages,
		Encoding: collector.EncodingType_BSON,
		Identity: buildIdentity(),
	}
	var err error
	start := time.Now()
	pm.Resp, err = c.PostMetrics(ctx, request)
	pm.rtt = time.Now().Sub(start)
	return err
}

// CallSummary returns a string representation of the RPC call result. It is
// mainly used for debug printing.
func (pm *PostMetricsMethod) CallSummary() string {
	rsp := fmt.Sprintf("%v %s", pm.Resp.Result, pm.Resp.Arg)
	return fmt.Sprintf("[%s] sent metrics, rtt=%v. rsp=%s", pm, pm.rtt, rsp)
}

// RetryOnErr denotes if retry is needed for this RPC method
func (pm *PostMetricsMethod) RetryOnErr() bool {
	return true
}

// PostStatusMethod is the struct for RPC method PostStatus
type PostStatusMethod struct {
	serviceKey string
	messages   [][]byte
	Resp       *collector.MessageResult
	rtt        time.Duration
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

// ResultCode returns the result code of the RPC call.
func (ps *PostStatusMethod) ResultCode() collector.ResultCode {
	return ps.Resp.Result
}

// Arg returns the Arg of the RPC call result.
func (ps *PostStatusMethod) Arg() string {
	return ps.Resp.Arg
}

// MessageLen returns the length of the RPC call message body
func (ps *PostStatusMethod) MessageLen() int64 {
	return int64(len(ps.messages))
}

// Message returns the RPC call message body
func (ps *PostStatusMethod) Message() [][]byte {
	return ps.messages
}

// Call issues an RPC call with the provided information.
func (ps *PostStatusMethod) Call(ctx context.Context,
	c collector.TraceCollectorClient) error {
	request := &collector.MessageRequest{
		ApiKey:   ps.serviceKey,
		Messages: ps.messages,
		Encoding: collector.EncodingType_BSON,
		Identity: buildIdentity(),
	}
	var err error
	start := time.Now()
	ps.Resp, err = c.PostStatus(ctx, request)
	ps.rtt = time.Now().Sub(start)
	return err
}

// RetryOnErr denotes if retry is needed for this RPC method
func (ps *PostStatusMethod) RetryOnErr() bool {
	return true
}

// CallSummary returns a string representation for the RPC call result. It is
// mainly used for debug printing.
func (ps *PostStatusMethod) CallSummary() string {
	rsp := fmt.Sprintf("%v %s", ps.Resp.Result, ps.Resp.Arg)
	return fmt.Sprintf("[%s] sent status, rtt=%v. rsp=%s", ps, ps.rtt, rsp)
}

// GetSettingsMethod is the struct for RPC method GetSettings
type GetSettingsMethod struct {
	serviceKey string
	Resp       *collector.SettingsResult
	rtt        time.Duration
}

func newGetSettingsMethod(key string) *GetSettingsMethod {
	return &GetSettingsMethod{
		serviceKey: key,
	}
}

func (gs *GetSettingsMethod) String() string {
	return "GetSettings"
}

// ResultCode returns the result code of the RPC call
func (gs *GetSettingsMethod) ResultCode() collector.ResultCode {
	return gs.Resp.Result
}

// Arg returns the Arg of the RPC call result
func (gs *GetSettingsMethod) Arg() string {
	return gs.Resp.Arg
}

// MessageLen returns the length of the RPC message body.
func (gs *GetSettingsMethod) MessageLen() int64 {
	return 0
}

// Message returns the message body of the RPC call.
func (gs *GetSettingsMethod) Message() [][]byte {
	return nil
}

// Call issues an RPC call.
func (gs *GetSettingsMethod) Call(ctx context.Context,
	c collector.TraceCollectorClient) error {
	request := &collector.SettingsRequest{
		ApiKey:        gs.serviceKey,
		ClientVersion: grpcReporterVersion,
		Identity:      buildBestEffortIdentity(),
	}
	var err error
	start := time.Now()
	gs.Resp, err = c.GetSettings(ctx, request)
	gs.rtt = time.Now().Sub(start)
	return err
}

// CallSummary returns a string representation of the RPC call result.
func (gs *GetSettingsMethod) CallSummary() string {
	rsp := fmt.Sprintf("%v %s", gs.Resp.Result, gs.Resp.Arg)
	return fmt.Sprintf("[%s] got settings, rtt=%v. rsp=%s", gs, gs.rtt, rsp)
}

// RetryOnErr denotes if this method needs a retry on failure.
func (gs *GetSettingsMethod) RetryOnErr() bool {
	return true
}

// PingMethod defines the RPC method `Ping`
type PingMethod struct {
	conn       string
	serviceKey string
	Resp       *collector.MessageResult
	rtt        time.Duration
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

// ResultCode returns the code of the RPC call result
func (p *PingMethod) ResultCode() collector.ResultCode {
	return p.Resp.Result
}

// Arg returns the arg of the RPC call result
func (p *PingMethod) Arg() string {
	return p.Resp.Arg
}

// MessageLen returns the length of the RPC call message body.
func (p *PingMethod) MessageLen() int64 {
	return 0
}

// Message returns the message body, if any.
func (p *PingMethod) Message() [][]byte {
	return nil
}

// Call makes an RPC call with the information provided.
func (p *PingMethod) Call(ctx context.Context,
	c collector.TraceCollectorClient) error {
	request := &collector.PingRequest{
		ApiKey: p.serviceKey,
	}
	var err error
	start := time.Now()
	p.Resp, err = c.Ping(ctx, request)
	p.rtt = time.Now().Sub(start)
	return err
}

// CallSummary returns a string representation for the RPC call result. It is
// mainly used for debug printing.
func (p *PingMethod) CallSummary() string {
	rsp := fmt.Sprintf("%v %s", p.Resp.Result, p.Resp.Arg)
	return fmt.Sprintf("[%s] ping back, rtt=%v. rsp=%s", p, p.rtt, rsp)
}

// RetryOnErr denotes if this RPC method needs a retry on failure.
func (p *PingMethod) RetryOnErr() bool {
	return false
}
