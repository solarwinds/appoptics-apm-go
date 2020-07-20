package lambda

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

type TIn struct {
	Val int
}

type TOut struct {
	Val int
}

type dumbWrapper struct {
	beforeIsCalled bool
	afterIsCalled  bool
}

func (d *dumbWrapper) before(ctx context.Context, msg json.RawMessage) context.Context {
	d.beforeIsCalled = true
	return ctx
}

func (d *dumbWrapper) after(interface{}, error, ...interface{}) {
	d.afterIsCalled = true
}

func TestWrapperInOut(t *testing.T) {
	fn := func(ctx context.Context, in *TIn) (*TOut, error) {
		return &TOut{Val: in.Val * 2}, nil
	}

	wr := &dumbWrapper{}
	fnW := handlerWrapper(fn, wr)
	fnWrapped := fnW.(func(ctx context.Context, message json.RawMessage) (interface{}, error))
	inBytes, _ := json.Marshal(&TIn{Val: 23})
	tOut, err := fnWrapped(context.Background(), inBytes)
	assert.Nil(t, err)
	assert.Equal(t, 23*2, tOut.(*TOut).Val) // assert that fn is called
	assert.True(t, wr.beforeIsCalled)
	assert.True(t, wr.afterIsCalled)
}

func TestWrapperIn(t *testing.T) {
	fn := func(ctx context.Context, in *TIn) error {
		return errors.New("fn error")
	}

	wr := &dumbWrapper{}
	fnW := handlerWrapper(fn, wr)
	fnWrapped := fnW.(func(ctx context.Context, message json.RawMessage) (interface{}, error))
	inBytes, _ := json.Marshal(&TIn{Val: 23})
	_, err := fnWrapped(context.Background(), inBytes)
	assert.NotNil(t, err)
	assert.True(t, wr.beforeIsCalled)
	assert.True(t, wr.afterIsCalled)
}

func TestWrapperInValid(t *testing.T) {
	fn := func(ctx context.Context, in *TIn, val int) (error, int) {
		return errors.New("fn error"), 0
	}

	wr := &dumbWrapper{}
	fnW := handlerWrapper(fn, wr)
	fnWrapped := fnW.(func(ctx context.Context, in *TIn, val int) (error, int))
	err, _ := fnWrapped(context.Background(), &TIn{Val: 23}, 0)
	assert.NotNil(t, err)
	assert.False(t, wr.beforeIsCalled)
	assert.False(t, wr.afterIsCalled)
}
