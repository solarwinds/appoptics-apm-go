package aogrpc

import (
	"testing"

	"github.com/appoptics/appoptics-apm-go/v1/ao"
	"github.com/appoptics/appoptics-apm-go/v1/contrib/aogrpc/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestGetTopFramePkg(t *testing.T) {
	// nil pointer
	pkg, err := getTopFramePkg(nil)
	assert.Equal(t, "", pkg)
	assert.NotNil(t, err)

	// returns nil
	m := mocks.StackTracer{}
	m.On("StackTrace").Return(nil)
	pkg, err = getTopFramePkg(&m)
	assert.Equal(t, "", pkg)
	assert.Equal(t, errEmptyStackTrace.Error(), err.Error())

	// returns empty frame stack
	m = mocks.StackTracer{}
	m.On("StackTrace").Return(errors.StackTrace{})
	pkg, err = getTopFramePkg(&m)
	assert.Equal(t, "", pkg)
	assert.Equal(t, errEmptyStackTrace.Error(), err.Error())

	// error from this package
	e := errors.Wrap(errors.New("inner error"), "wrapper")
	if ste, ok := e.(StackTracer); ok {
		pkg, err = getTopFramePkg(ste)
		assert.Equal(t, "aogrpc", pkg)
		assert.Nil(t, err)

		assert.Equal(t, "aogrpc", getErrClass(e))
	} else {
		assert.Equal(t, "error", getErrClass(e))
	}

	// error from another package
	e = ao.SetLogLevel("invalid_level")
	if ste, ok := e.(StackTracer); ok {
		pkg, err = getTopFramePkg(ste)
		assert.Equal(t, "ao", pkg)
		assert.Nil(t, err)

		assert.Equal(t, "ao", getErrClass(e))
	} else {
		assert.Equal(t, "error", getErrClass(e))
	}

}
