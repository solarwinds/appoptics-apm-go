package aogrpc

import (
	"io"
	"strings"
	"sync"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func actionFromMethod(method string) string {
	mParts := strings.Split(method, "/")

	return mParts[len(mParts)-1]
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

	t := ao.NewTraceFromID(serverName, xtID, func() ao.KVMap {
		return ao.KVMap{
			"Method":     "POST",
			"Controller": serverName,
			"Action":     action,
			"URL":        methodName,
			"Status":     statusCode,
		}
	})
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
		ctx, t = tracingContext(ctx, serverName, info.FullMethod, &statusCode)
		defer func() {
			t.SetStatus(statusCode)
			ao.EndTrace(ctx)
		}()
		resp, err = handler(ctx, req)
		if err != nil {
			statusCode = 500
			ao.Err(ctx, err)
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
		//	sp := ao.FromContext(newCtx)
		//	lg.Debug("server stream starting", "xtrace", sp.MetadataString())
		// }
		wrappedStream := wrapServerStream(stream)
		wrappedStream.WrappedContext = newCtx
		err = handler(srv, wrappedStream)
		if err == io.EOF {
			return nil
		} else if err != nil {
			statusCode = 500
			ao.Err(newCtx, err)
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
			span.Err(err)
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
		span.Err(err)
	}
	span.End()
}
