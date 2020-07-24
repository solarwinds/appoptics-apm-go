package lambda

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/pkg/errors"

	"github.com/appoptics/appoptics-apm-go/v1/ao"
)

type Wrapper interface {
	Before(context.Context, json.RawMessage) context.Context
	After(interface{}, error, ...interface{})
}

type traceWrapper struct {
	trace ao.Trace
}

func (w *traceWrapper) Before(ctx context.Context, msg json.RawMessage) context.Context {
	lc, _ := lambdacontext.FromContext(ctx)
	awsRequestID := "not_found"
	if lc != nil {
		awsRequestID = lc.AwsRequestID
	}

	w.trace = ao.NewTraceWithOptions("aws_lambda",
		ao.SpanOptions{
			ContextOptions: ao.ContextOptions{
				LambdaRequestID: awsRequestID,
			},
		},
	)
	return ao.NewContext(ctx, w.trace)
}

func (w *traceWrapper) After(result interface{}, err error, endArgs ...interface{}) {
	if err != nil {
		w.trace.Err(err)
	}
	defer w.trace.End(endArgs...)

	if panicErr := recover(); panicErr != nil {
		w.trace.Error("panic", fmt.Sprintf("%v", panicErr))
		panic(panicErr) // re-raise the panic
	}
}

// Wrap wraps the AWS Lambda handler and do the instrumentation. It
// returns a new handler and can be passed into lambda.Start()
func Wrap(handlerFunc interface{}) interface{} {
	return HandlerWithWrapper(handlerFunc, &traceWrapper{})
}

func HandlerWithWrapper(handlerFunc interface{}, w Wrapper) interface{} {
	if err := checkSignature(reflect.TypeOf(handlerFunc)); err != nil {
		// Does not wrap an invalid handler but let lambda.Start() reject it later
		return handlerFunc
	}

	var endArgs []interface{}
	if f := runtime.FuncForPC(reflect.ValueOf(handlerFunc).Pointer()); f != nil {
		// e.g. "main.slowHandler", "github.com/appoptics/appoptics-apm-go/v1/ao_test.handler404"
		fname := f.Name()
		if s := strings.SplitN(fname[strings.LastIndex(fname, "/")+1:], ".", 2); len(s) == 2 {
			endArgs = append(endArgs, "Controller", s[0], "Action", s[1])
		}
	}

	return func(ctx context.Context, msg json.RawMessage) (interface{}, error) {
		var result interface{}
		var err error

		ctx = w.Before(ctx, msg)
		defer func() {
			w.After(result, err, endArgs...)
		}()

		result, err = callHandler(ctx, msg, handlerFunc)

		return result, err
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
