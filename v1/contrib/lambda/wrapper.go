package lambda

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/pkg/errors"

	"github.com/appoptics/appoptics-apm-go/v1/ao"
)

const (
	AOHTTPHeader = "x-trace"
)

// Wrapper offers the ability to be called before and after a handler is executed.
type Wrapper interface {
	// Before is called before the real handler is called.
	Before(context.Context, json.RawMessage, ...interface{}) context.Context
	// After is called after the real handler is called.
	After(interface{}, *typedError, ...interface{}) interface{}
}

type traceWrapper struct {
	trace ao.Trace
	mdStr string
}

type eventHeaders struct {
	Headers    map[string]string `json:"headers,omitempty"`
	HTTPMethod string            `json:"httpMethod,omitempty"`
}

func (w *traceWrapper) getRequestContext(ctx context.Context, msg json.RawMessage, args ...interface{}) (string, string, []interface{}) {
	evtv1 := &events.APIGatewayProxyRequest{}
	evtv2 := &events.APIGatewayV2HTTPRequest{}
	evt := &eventHeaders{}
	var headers map[string]string
	var method string
	var mdStr string
	var host string

	if json.Unmarshal(msg, evtv2) == nil && evtv2.Version == "2.0" {
		method = evtv2.RequestContext.HTTP.Method
		headers = evtv2.Headers
		args = append(args, "HTTPMethod", method)
		args = append(args, "URL", evtv2.RequestContext.HTTP.Path)
		args = append(args, "APIGatewayResourceID", evtv2.RouteKey)
		args = append(args, "APIGatewayStage", evtv2.RequestContext.Stage)
	} else if json.Unmarshal(msg, evtv1) == nil {
		method = evtv1.HTTPMethod
		headers = evtv1.Headers
		args = append(args, "HTTPMethod", method)
		args = append(args, "URL", evtv1.Path)
		args = append(args, "APIGatewayResourceID", evtv1.RequestContext.ResourceID)
		args = append(args, "APIGatewayStage", evtv1.RequestContext.Stage)
	} else if json.Unmarshal(msg, evt) == nil {
		method = evt.HTTPMethod
		headers = evt.Headers
		args = append(args, "Method", method)
	}

	if headers != nil {
		for k, v := range headers {
			k = strings.ToLower(k)
			switch k {
			case AOHTTPHeader:
				mdStr = v
			case "host":
				host = v
				args = append(args, "HTTP-Host", v)
			case "x-forwarded-for":
				args = append(args, "Forwarded-For", v)
			case "x-forwarded-proto":
				args = append(args, "Forwarded-Proto", v)
			case "x-forwarded-port":
				args = append(args, "Forwarded-Port", v)
			}
		}
	}

	if method != "" && host != "" {
		args = append(args, "Spec", "aws-lambda:ws")
	} else {
		args = append(args, "Spec", "aws-lambda")
	}

	return mdStr, method, args
}

func (w *traceWrapper) Before(ctx context.Context, msg json.RawMessage, args ...interface{}) context.Context {
	mdStr, method, args := w.getRequestContext(ctx, msg, args...)

	lc, ok := lambdacontext.FromContext(ctx)
	if ok {
		args = append(args, "AwsRequestID", lc.AwsRequestID)
		args = append(args, "InvokedFunctionArn", lc.InvokedFunctionArn)
		args = append(args, "FunctionName", lambdacontext.FunctionName)
		args = append(args, "FunctionVersion", lambdacontext.FunctionVersion)
		args = append(args, "AWSRegion", os.Getenv("AWS_REGION"))
		args = append(args, "MemoryLimitInMB", lambdacontext.MemoryLimitInMB)
		args = append(args, "LogStreamName", lambdacontext.LogStreamName)
	}

	cb := func() ao.KVMap {
		m := ao.KVMap{}
		for i := 0; i < len(args)-1; i += 2 {
			if k, ok := args[i].(string); ok {
				m[k] = args[i+1]
			}
		}
		return m
	}

	w.trace = ao.NewTraceWithOptions("aws_lambda_go",
		ao.SpanOptions{
			ContextOptions: ao.ContextOptions{
				MdStr: mdStr,
				CB:    cb,
			},
		},
	)
	w.mdStr = mdStr
	txnName := strings.TrimLeft(method+"."+lambdacontext.FunctionName, ".")
	w.trace.SetTransactionName(txnName)
	w.trace.SetMethod(method)
	return ao.NewContext(ctx, w.trace)
}

type typedError struct {
	typ string
	err error
}

func (w *traceWrapper) After(result interface{}, err *typedError, endArgs ...interface{}) interface{} {
	statusCode := 0

	if err != nil {
		w.trace.Error(err.typ, err.err.Error())
		statusCode = 500
	}

	if statusCode == 0 {
		if gwRsp, ok := result.(events.APIGatewayProxyResponse); ok {
			statusCode = gwRsp.StatusCode
		} else if gwRsp, ok := result.(events.APIGatewayV2HTTPResponse); ok {
			statusCode = gwRsp.StatusCode
		}
	}

	if statusCode != 0 {
		endArgs = append(endArgs, "Status", statusCode)
	}

	defer w.trace.End(endArgs...)
	if w.mdStr != "" {
		return w.injectTraceContext(result, w.trace.ExitMetadata())
	} else {
		return result
	}
}

func (w *traceWrapper) injectTraceContext(result interface{}, mdStr string) (res interface{}) {
	defer func() {
		if err := recover(); err != nil {
			res = result
		}
	}()

	if gwRsp, ok := result.(events.APIGatewayProxyResponse); ok {
		if gwRsp.Headers != nil {
			gwRsp.Headers[AOHTTPHeader] = mdStr
			return result
		}
	} else if gwRsp, ok := result.(events.APIGatewayV2HTTPResponse); ok {
		if gwRsp.Headers != nil {
			gwRsp.Headers[AOHTTPHeader] = mdStr
			return result
		}
	} else {
		return result
	}

	rVal := reflect.Indirect(reflect.ValueOf(result))
	rType := rVal.Type()
	if rVal.Kind() != reflect.Struct {
		return result
	}

	newResult := reflect.New(rType).Elem()

	for i := 0; i < newResult.NumField(); i++ {
		fieldVal := reflect.Indirect(newResult.Field(i))
		fieldType := newResult.Type().Field(i)

		fieldVal.Set(rVal.Field(i))
		if fieldType.Name == "Headers" &&
			fieldType.Type.Kind() == reflect.Map &&
			fieldType.Type.Elem().Kind() == reflect.String &&
			fieldType.Type.Key().Kind() == reflect.String &&
			fieldVal.CanSet() {
			traceContextMap := map[string]string{AOHTTPHeader: mdStr}
			fieldVal.Set(reflect.Indirect(reflect.ValueOf(traceContextMap)))
		}
	}

	return newResult.Interface()
}

// Wrap wraps the AWS Lambda handler and do the instrumentation. It
// returns a new handler and can be passed into lambda.Start()
func Wrap(handlerFunc interface{}) interface{} {
	return HandlerWithWrapper(handlerFunc, &traceWrapper{})
}

// HandlerWithWrapper wraps a function with the wrapper and returns the wrapped function.
func HandlerWithWrapper(handlerFunc interface{}, w Wrapper) interface{} {
	if err := checkSignature(reflect.TypeOf(handlerFunc)); err != nil {
		// Does not wrap an invalid handler but let lambda.Start() reject it later
		return handlerFunc
	}

	invocationCount := 1
	var endArgs []interface{}
	if f := runtime.FuncForPC(reflect.ValueOf(handlerFunc).Pointer()); f != nil {
		// e.g. "main.slowHandler", "github.com/appoptics/appoptics-apm-go/v1/ao_test.handler404"
		fname := f.Name()
		if s := strings.SplitN(fname[strings.LastIndex(fname, "/")+1:], ".", 2); len(s) == 2 {
			endArgs = append(endArgs, "Controller", s[0], "Action", s[1])
		}
	}

	return func(ctx context.Context, msg json.RawMessage) (res interface{}, err error) {
		ctx = w.Before(ctx, msg, "InvocationCount", invocationCount)
		defer func() {
			var panicErr interface{}
			var te *typedError

			if panicErr = recover(); panicErr != nil {
				te = &typedError{typ: "panic", err: fmt.Errorf("%v", panicErr)}
			} else if err != nil {
				te = &typedError{typ: "error", err: err}
			}

			res = w.After(res, te, endArgs...)
			if panicErr != nil {
				panic(panicErr)
			}
		}()

		res, err = callHandler(ctx, msg, handlerFunc)
		invocationCount++

		return res, err
	}
}

func callHandler(ctx context.Context, msg json.RawMessage, handler interface{}) (interface{}, error) {
	ev, err := unmarshalEvent(msg, reflect.TypeOf(handler))
	if err != nil {
		return nil, err
	}

	handlerType := reflect.TypeOf(handler)
	// the arguments to call the handler
	var args []reflect.Value

	if handlerType.NumIn() == 1 {
		if handlerType.In(0).Implements(reflect.TypeOf((*context.Context)(nil)).Elem()) {
			args = []reflect.Value{reflect.ValueOf(ctx)}
		} else {
			args = []reflect.Value{ev.Elem()}

		}
	} else if handlerType.NumIn() == 2 {
		args = []reflect.Value{reflect.ValueOf(ctx), ev.Elem()}
	}

	ret := reflect.ValueOf(handler).Call(args)

	var rsp interface{}

	if len(ret) > 0 {
		val := ret[len(ret)-1].Interface()
		if errVal, ok := val.(error); ok {
			err = errVal
		}
	}

	if len(ret) > 1 {
		rsp = ret[0].Interface()
	}

	return rsp, err
}

func unmarshalEvent(ev json.RawMessage, handlerType reflect.Type) (reflect.Value, error) {
	if handlerType.NumIn() == 0 {
		return reflect.ValueOf(nil), nil
	}

	if handlerType.NumIn() == 1 &&
		handlerType.In(0).Implements(reflect.TypeOf((*context.Context)(nil)).Elem()) {
		return reflect.ValueOf(nil), nil
	}

	newMessage := reflect.New(handlerType.In(handlerType.NumIn() - 1))
	err := json.Unmarshal(ev, newMessage.Interface())
	if err != nil {
		return reflect.ValueOf(nil), err
	}
	return newMessage, err
}

func checkSignature(handler reflect.Type) error {
	// check type
	if handler.Kind() != reflect.Func {
		return fmt.Errorf("handler kind %s is not %s", handler.Kind(), reflect.Func)
	}

	// check parameters
	if handler.NumIn() > 2 {
		return fmt.Errorf("handler takes too many arguments: %d", handler.NumIn())
	}

	if handler.NumIn() == 2 {
		if !handler.In(0).Implements(reflect.TypeOf((*context.Context)(nil)).Elem()) {
			return errors.New("context should be the first argument")
		}
	}

	// check return values
	if handler.NumOut() > 2 {
		return fmt.Errorf("handler returns too many values: %d", handler.NumOut())
	}

	if handler.NumOut() > 0 {
		rt := handler.Out(handler.NumOut() - 1)
		if !rt.Implements(reflect.TypeOf((*error)(nil)).Elem()) {
			return errors.New("handler should return error as the last value")
		}
	}
	return nil
}
