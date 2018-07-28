// +build linux

// Copyright (c) 2017 Librato, Inc. All rights reserved.

package host

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDistro(t *testing.T) {
	distro := strings.ToLower(getDistro())

	assert.NotEmpty(t, distro)

	if runtime.GOOS == "linux" {
		assert.NotContains(t, distro, "unknown")
	} else {
		assert.Contains(t, distro, "unknown")
	}
}
