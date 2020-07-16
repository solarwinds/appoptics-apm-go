package lambda

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/pkg/errors"

	"github.com/appoptics/appoptics-apm-go/v1/ao"
)

// HandlerWrapper wraps the AWS Lambda handler and do the instrumentation. It
// returns a new handler and can be passed into lambda.Start()
func HandlerWrapper(handlerFunc interface{}) interface{} {
	handlerType := reflect.TypeOf(handlerFunc)
	if err := checkSignature(handlerType); err != nil {
		// Does not wrap an invalid handler
		return handlerFunc
	}

	return func(ctx context.Context, msg json.RawMessage) (interface{}, error) {
		lc, _ := lambdacontext.FromContext(ctx)
		defer ao.NewTraceWithOptions("aws_lambda",
			ao.SpanOptions{
				ContextOptions: ao.ContextOptions{
					LambdaRequestID: lc.AwsRequestID,
				},
			},
		).End()
		result, err := callHandler(ctx, msg, handlerFunc)
		return result, err
	}
}

func callHandler(ctx context.Context, msg json.RawMessage, handler interface{}) (interface{}, error) {
	ev, err := unmarshalEvent(msg, reflect.TypeOf(handler))
	if err != nil {
		return nil, err
	}
	handlerType := reflect.TypeOf(handler)

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

	messageType := handlerType.In(handlerType.NumIn() - 1)
	contextType := reflect.TypeOf((*context.Context)(nil)).Elem()
	firstArgType := handlerType.In(0)

	if handlerType.NumIn() == 1 && firstArgType.Implements(contextType) {
		return reflect.ValueOf(nil), nil
	}

	newMessage := reflect.New(messageType)
	err := json.Unmarshal(ev, newMessage.Interface())
	if err != nil {
		return reflect.ValueOf(nil), err
	}
	return newMessage, err
}

func checkSignature(handler reflect.Type) error {
	if handler.Kind() != reflect.Func {
		return fmt.Errorf("handler kind %s is not %s", handler.Kind(), reflect.Func)
	}

	if handler.NumIn() > 2 {
		return fmt.Errorf("handler takes too many arguments: %d", handler.NumIn())
	}

	if handler.NumIn() == 2 {
		contextType := reflect.TypeOf((*context.Context)(nil)).Elem()
		firstArgType := handler.In(0)
		if !firstArgType.Implements(contextType) {
			return errors.New("context should be the first argument")
		}
	}

	errorType := reflect.TypeOf((*error)(nil)).Elem()

	if handler.NumOut() > 2 {
		return fmt.Errorf("handler returns too many values: %d", handler.NumOut())
	}

	if handler.NumOut() > 0 {
		rt := handler.Out(handler.NumOut() - 1)
		if !rt.Implements(errorType) {
			return errors.New("handler should return error as the last value")
		}
	}
	return nil
}
