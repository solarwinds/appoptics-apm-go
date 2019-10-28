// Copyright (C) 2018 Librato, Inc. All rights reserved.

package reporter

import (
	"context"
	"fmt"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter/collector"
	"github.com/pkg/errors"
)

// Method defines the interface of an RPC call
type Method interface {
	// Call invokes the RPC method through a specified gRPC client. It may return
	// an error if the call is failed and no result code is returned by the collector
	// due to some reason, e.g., the transport issue.
	Call(ctx context.Context, c collector.TraceCollectorClient) error

	// CallSummary returns a string indicating the summary of this call, e.g., the
	// data chunks sent out and the round trip time.It is for logging purpose only.
	CallSummary() string

	// MessageLen returns the length (number of elements) of the method request
	MessageLen() int64

	// RequestSize returns the total bytes for an rpc request body
	RequestSize() int64

	// Message returns the message body, if any
	Message() [][]byte

	// ResultCode is the return code of the RPC call. It must only be called after
	// the Call() method is invoked.
	ResultCode() (collector.ResultCode, error)

	// Arg returns the Arg field of the RPC call result. It must only be called
	// after the Call() method is invoked.
	Arg() string

	// String returns the string representation of the Method object
	String() string

	// RetryOnErr checks if the method allows retry
	RetryOnErr(error) bool
}

var (
	errRPCNotIssued = errors.New("RPC request is not issued yet")
	errGotNilResp   = errors.New("got nil resp from collector")
)

// PostEventsMethod is the struct for RPC method PostEvents
type PostEventsMethod struct {
	serviceKey string
	messages   [][]byte
	msgSize    int64
	Resp       *collector.MessageResult
	err        error
	rtt        time.Duration
}

func newPostEventsMethod(key string, msgs [][]byte) *PostEventsMethod {
	return &PostEventsMethod{
		serviceKey: key,
		messages:   msgs,
		Resp:       &collector.MessageResult{},
		err:        errRPCNotIssued,
	}
}

func (pe *PostEventsMethod) String() string {
	return "PostEvents"
}

// ResultCode returns the result code of the PostEvents RPC call. This method
// should only be invoked after the method Call is called.
func (pe *PostEventsMethod) ResultCode() (collector.ResultCode, error) {
	return pe.Resp.Result, pe.err
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

// RequestSize returns the total bytes for an rpc request body
func (pe *PostEventsMethod) RequestSize() int64 {
	if pe.msgSize != 0 {
		return pe.msgSize
	}
	pe.msgSize = getMsgsBytes(pe.messages)
	return pe.msgSize
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
	start := time.Now()
	pe.Resp, pe.err = c.PostEvents(ctx, request)
	pe.rtt = time.Now().Sub(start)
	return pe.err
}

// CallSummary returns a string representation of the RPC call result.
// It is mainly used for debug printing.
func (pe *PostEventsMethod) CallSummary() string {
	rsp := resultRespStr(pe.Resp, pe.err)
	return fmt.Sprintf("[%s] sent %d events, %d bytes, rtt=%v, rsp=%s.",
		pe, pe.MessageLen(), pe.RequestSize(), pe.rtt, rsp)
}

// RetryOnErr denotes if retry is needed for this RPC method
func (pe *PostEventsMethod) RetryOnErr(err error) bool {
	if errRequestTooBig == errors.Cause(err) {
		return false
	}
	return true
}

// PostMetricsMethod is the struct for RPC method PostMetrics
type PostMetricsMethod struct {
	serviceKey string
	messages   [][]byte
	msgSize    int64
	Resp       *collector.MessageResult
	err        error
	rtt        time.Duration
}

func newPostMetricsMethod(key string, msgs [][]byte) *PostMetricsMethod {
	return &PostMetricsMethod{
		serviceKey: key,
		messages:   msgs,
		Resp:       &collector.MessageResult{},
		err:        errRPCNotIssued,
	}
}

func (pm *PostMetricsMethod) String() string {
	return "PostMetrics"
}

// ResultCode returns the result code of the RPC call. This method should
// only be called after the method Call is invoked.
func (pm *PostMetricsMethod) ResultCode() (collector.ResultCode, error) {
	return pm.Resp.Result, pm.err
}

// Arg returns the Arg of the RPC call result
func (pm *PostMetricsMethod) Arg() string {
	return pm.Resp.Arg
}

// MessageLen returns the length of the RPC call message.
func (pm *PostMetricsMethod) MessageLen() int64 {
	return int64(len(pm.messages))
}

// RequestSize returns the total bytes for an rpc request body
func (pm *PostMetricsMethod) RequestSize() int64 {
	if pm.msgSize != 0 {
		return pm.msgSize
	}
	pm.msgSize = getMsgsBytes(pm.messages)
	return pm.msgSize
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
	start := time.Now()
	pm.Resp, pm.err = c.PostMetrics(ctx, request)
	pm.rtt = time.Now().Sub(start)
	return pm.err
}

// CallSummary returns a string representation of the RPC call result. It is
// mainly used for debug printing.
func (pm *PostMetricsMethod) CallSummary() string {
	rsp := resultRespStr(pm.Resp, pm.err)
	return fmt.Sprintf("[%s] sent %d metrics, %d bytes, rtt=%v, rsp=%s.",
		pm, pm.MessageLen(), pm.RequestSize(), pm.rtt, rsp)
}

// RetryOnErr denotes if retry is needed for this RPC method
func (pm *PostMetricsMethod) RetryOnErr(err error) bool {
	if errRequestTooBig == errors.Cause(err) {
		return false
	}
	return true
}

// PostStatusMethod is the struct for RPC method PostStatus
type PostStatusMethod struct {
	serviceKey string
	messages   [][]byte
	msgSize    int64
	Resp       *collector.MessageResult
	err        error
	rtt        time.Duration
}

func newPostStatusMethod(key string, msgs [][]byte) *PostStatusMethod {
	return &PostStatusMethod{
		serviceKey: key,
		messages:   msgs,
		Resp:       &collector.MessageResult{},
		err:        errRPCNotIssued,
	}
}

func (ps *PostStatusMethod) String() string {
	return "PostStatus"
}

// ResultCode returns the result code of the RPC call.
func (ps *PostStatusMethod) ResultCode() (collector.ResultCode, error) {
	return ps.Resp.Result, ps.err
}

// Arg returns the Arg of the RPC call result.
func (ps *PostStatusMethod) Arg() string {
	return ps.Resp.Arg
}

// MessageLen returns the length of the RPC call message body
func (ps *PostStatusMethod) MessageLen() int64 {
	return int64(len(ps.messages))
}

// RequestSize returns the total bytes for an rpc request body
func (ps *PostStatusMethod) RequestSize() int64 {
	if ps.msgSize != 0 {
		return ps.msgSize
	}

	ps.msgSize = getMsgsBytes(ps.messages)
	return ps.msgSize
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
	start := time.Now()
	ps.Resp, ps.err = c.PostStatus(ctx, request)
	ps.rtt = time.Now().Sub(start)
	return ps.err
}

// RetryOnErr denotes if retry is needed for this RPC method
func (ps *PostStatusMethod) RetryOnErr(err error) bool {
	if errRequestTooBig == errors.Cause(err) {
		return false
	}
	return true
}

// CallSummary returns a string representation for the RPC call result. It is
// mainly used for debug printing.
func (ps *PostStatusMethod) CallSummary() string {
	rsp := resultRespStr(ps.Resp, ps.err)
	return fmt.Sprintf("[%s] sent status, rtt=%v, rsp=%s.", ps, ps.rtt, rsp)
}

// GetSettingsMethod is the struct for RPC method GetSettings
type GetSettingsMethod struct {
	serviceKey string
	Resp       *collector.SettingsResult
	err        error
	rtt        time.Duration
}

func newGetSettingsMethod(key string) *GetSettingsMethod {
	return &GetSettingsMethod{
		serviceKey: key,
		Resp:       &collector.SettingsResult{},
		err:        errRPCNotIssued,
	}
}

func (gs *GetSettingsMethod) String() string {
	return "GetSettings"
}

// ResultCode returns the result code of the RPC call
func (gs *GetSettingsMethod) ResultCode() (collector.ResultCode, error) {
	return gs.Resp.Result, gs.err
}

// Arg returns the Arg of the RPC call result
func (gs *GetSettingsMethod) Arg() string {
	return gs.Resp.Arg
}

// MessageLen returns the length of the RPC message body.
func (gs *GetSettingsMethod) MessageLen() int64 {
	return 0
}

// RequestSize returns the total bytes for an rpc request body
func (gs *GetSettingsMethod) RequestSize() int64 {
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
	start := time.Now()
	gs.Resp, gs.err = c.GetSettings(ctx, request)
	gs.rtt = time.Now().Sub(start)
	return gs.err
}

// CallSummary returns a string representation of the RPC call result.
func (gs *GetSettingsMethod) CallSummary() string {
	rsp := settingsRespStr(gs.Resp, gs.err)
	return fmt.Sprintf("[%s] got settings, rtt=%v, rsp=%s.", gs, gs.rtt, rsp)
}

// RetryOnErr denotes if this method needs a retry on failure.
func (gs *GetSettingsMethod) RetryOnErr(err error) bool {
	return true
}

// PingMethod defines the RPC method `Ping`
type PingMethod struct {
	conn       string
	serviceKey string
	Resp       *collector.MessageResult
	err        error
	rtt        time.Duration
}

func newPingMethod(key string, conn string) *PingMethod {
	return &PingMethod{
		serviceKey: key,
		conn:       conn,
		Resp:       &collector.MessageResult{},
		err:        errRPCNotIssued,
	}
}

func (p *PingMethod) String() string {
	return "Ping " + p.conn
}

// ResultCode returns the code of the RPC call result
func (p *PingMethod) ResultCode() (collector.ResultCode, error) {
	return p.Resp.Result, p.err
}

// Arg returns the arg of the RPC call result
func (p *PingMethod) Arg() string {
	return p.Resp.Arg
}

// MessageLen returns the length of the RPC call message body.
func (p *PingMethod) MessageLen() int64 {
	return 0
}

// RequestSize returns the total bytes for an rpc request body
func (p *PingMethod) RequestSize() int64 {
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
	start := time.Now()
	p.Resp, p.err = c.Ping(ctx, request)
	p.rtt = time.Now().Sub(start)
	return p.err
}

// CallSummary returns a string representation for the RPC call result. It is
// mainly used for debug printing.
func (p *PingMethod) CallSummary() string {
	rsp := resultRespStr(p.Resp, p.err)
	return fmt.Sprintf("[%s] ping back, rtt=%v, rsp=%s.", p, p.rtt, rsp)
}

// RetryOnErr denotes if this RPC method needs a retry on failure.
func (p *PingMethod) RetryOnErr(err error) bool {
	return false
}

func resultRespStr(r *collector.MessageResult, err error) string {
	if err != nil {
		return err.Error()
	}
	if r == nil {
		return errGotNilResp.Error()
	}
	return fmt.Sprintf("%v %s", r.Result, r.Arg)
}

func settingsRespStr(r *collector.SettingsResult, err error) string {
	if err != nil {
		return err.Error()
	}
	if r == nil {
		return errGotNilResp.Error()
	}
	return fmt.Sprintf("%v %s", r.Result, r.Arg)
}

// getMsgsBytes is a internal helper function to calculate the total bytes of a
// message slice.
func getMsgsBytes(msg [][]byte) int64 {
	size := 0
	for i := 0; i < len(msg); i++ {
		size += len(msg[i])
	}
	return int64(size)
}
