// Copyright (C) 2016 Librato, Inc. All rights reserved.

package ao

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSetGetLogLevel(t *testing.T) {
	oldLevel := GetLogLevel()

	err := SetLogLevel("INVALID")
	assert.Equal(t, err, errInvalidLogLevel)

	nl := "ERROR"
	err = SetLogLevel(nl)
	assert.Nil(t, err)

	newLevel := GetLogLevel()
	assert.Equal(t, newLevel, nl)

	SetLogLevel(oldLevel)
}

func TestShutdown(t *testing.T) {
	Shutdown(context.Background())
	assert.True(t, Closed())
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour*24)
	defer cancel()
	assert.False(t, WaitForReady(ctx))
}
