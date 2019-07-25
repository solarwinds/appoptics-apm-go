package aogrpc

import (
	"fmt"
	"io"
	fp "path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/appoptics/appoptics-apm-go/v1/ao"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func actionFromMethod(method string) string {
	mParts := strings.Split(method, "/")

	return mParts[len(mParts)-1]
}

// StackTracer is a copy of the stackTracer interface of pkg/errors.
//
// This may be fragile as stackTracer is not imported, just try our best though.
type StackTracer interface {
	StackTrace() errors.StackTrace
}

func getErrClass(err error) string {
	if st, ok := err.(StackTracer); ok {
		pkg, e := getTopFramePkg(st)
		if e == nil {
			return pkg
		}
	}
	// seems we cannot do anything else, so just return the fallback value
	return "error"
}

var (
	errNilStackTracer  = errors.New("nil stackTracer pointer")
	errEmptyStackTrace = errors.New("empty stack trace")
	errGetTopFramePkg  = errors.New("failed to get top frame package name")
)

func getTopFramePkg(st StackTracer) (string, error) {
	if st == nil {
		return "", errNilStackTracer
	}
	trace := st.StackTrace()
	if len(trace) == 0 {
		return "", errEmptyStackTrace
	}
	fs := fmt.Sprintf("%+s", trace[0])
	// it is fragile to use this hard-coded separator
	// see: https://github.com/pkg/errors/blob/30136e27e2ac8d167177e8a583aa4c3fea5be833/stack.go#L63
	frames := strings.Split(fs, "\n\t")
	if len(frames) != 2 {
		return "", errGetTopFramePkg
	}
	return fp.Base(fp.Dir(frames[1])), nil
}

func tracingContext(ctx context.Context, serverName string, methodName string, statusCode *int) (context.Context, ao.Trace) {

	action := actionFromMethod(methodName)

	xtID := ""
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if xt, ok := md[ao.HTTPHeaderName]; ok {
			xtID = xt[0]
		} else if xt, ok = md[strings.ToLower(ao.HTTPHeaderName)]; ok {
			xtID = xt[0]
		}
	}

	triggerTrace, triggerTraceKVs := ao.CheckTriggerTraceHeader(md)

	t := ao.NewTraceWithOptions(serverName, ao.SpanOptions{
		ContextOptions: ao.ContextOptions{
			MdStr:        xtID,
			URL:          methodName,
			TriggerTrace: triggerTrace,
			CB: func() ao.KVMap {
				kvs := ao.KVMap{
					"Method":     "POST",
					"Controller": serverName,
					"Action":     action,
					"URL":        methodName,
					"Status":     statusCode,
				}

				for k, v := range triggerTraceKVs {
					kvs[k] = v
				}
				return kvs
			},
		}})

	t.SetMethod("POST")
	t.SetTransactionName(serverName + "." + action)
	t.SetStartTime(time.Now())

	return ao.NewContext(ctx, t), t
}

// UnaryServerInterceptor returns an interceptor that traces gRPC unary server RPCs using AppOptics.
// If the client is using UnaryClientInterceptor, the distributed trace's context will be read from the client.
func UnaryServerInterceptor(serverName string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		var err error
		var resp interface{}
		var statusCode = 200
		var t ao.Trace
		ctx, t = tracingContext(ctx, serverName, info.FullMethod, &statusCode)
		defer func() {
			t.SetStatus(statusCode)
			ao.EndTrace(ctx)
		}()
		resp, err = handler(ctx, req)
		if err != nil {
			statusCode = 500
			ao.Error(ctx, getErrClass(err), err.Error())
		}
		return resp, err
	}
}

// wrappedServerStream from the grpc_middleware project
type wrappedServerStream struct {
	grpc.ServerStream
	WrappedContext context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.WrappedContext
}

func wrapServerStream(stream grpc.ServerStream) *wrappedServerStream {
	if existing, ok := stream.(*wrappedServerStream); ok {
		return existing
	}
	return &wrappedServerStream{ServerStream: stream, WrappedContext: stream.Context()}
}

// StreamServerInterceptor returns an interceptor that traces gRPC streaming server RPCs using AppOptics.
// Each server span starts with the first message and ends when all request and response messages have finished streaming.
func StreamServerInterceptor(serverName string) grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		var err error
		var statusCode = 200
		newCtx, t := tracingContext(stream.Context(), serverName, info.FullMethod, &statusCode)
		defer func() {
			t.SetStatus(statusCode)
			ao.EndTrace(newCtx)
		}()
		// if lg.IsDebug() {
		// 	sp := ao.FromContext(newCtx)
		// 	lg.Debug("server stream starting", "xtrace", sp.MetadataString())
		// }
		wrappedStream := wrapServerStream(stream)
		wrappedStream.WrappedContext = newCtx
		err = handler(srv, wrappedStream)
		if err == io.EOF {
			return nil
		} else if err != nil {
			statusCode = 500
			ao.Error(newCtx, getErrClass(err), err.Error())
		}
		return err
	}
}

// UnaryClientInterceptor returns an interceptor that traces a unary RPC from a gRPC client to a server using
// AppOptics, by propagating the distributed trace's context from client to server using gRPC metadata.
func UnaryClientInterceptor(target string, serviceName string) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, resp interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		action := actionFromMethod(method)
		span := ao.BeginRPCSpan(ctx, action, "grpc", serviceName, target)
		defer span.End()
		xtID := span.MetadataString()
		if len(xtID) > 0 {
			ctx = metadata.AppendToOutgoingContext(ctx, ao.HTTPHeaderName, xtID)
		}
		err := invoker(ctx, method, req, resp, cc, opts...)
		if err != nil {
			span.Error(getErrClass(err), err.Error())
			return err
		}
		return nil
	}
}

// StreamClientInterceptor returns an interceptor that traces a streaming RPC from a gRPC client to a server using
// AppOptics, by propagating the distributed trace's context from client to server using gRPC metadata.
// The client span starts with the first message and ends when all request and response messages have finished streaming.
func StreamClientInterceptor(target string, serviceName string) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		action := actionFromMethod(method)
		span := ao.BeginRPCSpan(ctx, action, "grpc", serviceName, target)
		xtID := span.MetadataString()
		// lg.Debug("stream client interceptor", "x-trace", xtID)
		if len(xtID) > 0 {
			ctx = metadata.AppendToOutgoingContext(ctx, ao.HTTPHeaderName, xtID)
		}
		clientStream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			closeSpan(span, err)
			return nil, err
		}
		return &tracedClientStream{ClientStream: clientStream, span: span}, nil
	}
}

type tracedClientStream struct {
	grpc.ClientStream
	mu     sync.Mutex
	closed bool
	span   ao.Span
}

func (s *tracedClientStream) Header() (metadata.MD, error) {
	h, err := s.ClientStream.Header()
	if err != nil {
		s.closeSpan(err)
	}
	return h, err
}

func (s *tracedClientStream) SendMsg(m interface{}) error {
	err := s.ClientStream.SendMsg(m)
	if err != nil {
		s.closeSpan(err)
	}
	return err
}

func (s *tracedClientStream) CloseSend() error {
	err := s.ClientStream.CloseSend()
	if err != nil {
		s.closeSpan(err)
	}
	return err
}

func (s *tracedClientStream) RecvMsg(m interface{}) error {
	err := s.ClientStream.RecvMsg(m)
	if err != nil {
		s.closeSpan(err)
	}
	return err
}

func (s *tracedClientStream) closeSpan(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		closeSpan(s.span, err)
		s.closed = true
	}
}

func closeSpan(span ao.Span, err error) {
	// lg.Debug("closing span", "err", err.Error())
	if err != nil && err != io.EOF {
		span.Error(getErrClass(err), err.Error())
	}
	span.End()
}
