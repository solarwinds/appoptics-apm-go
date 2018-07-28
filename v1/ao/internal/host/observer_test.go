// Copyright (c) 2017 Librato, Inc. All rights reserved.

package host

import (
	"fmt"
	"net"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitContainerID(t *testing.T) {
	id := "unintialized"

	var mockGetContainerMeta = func(keyword string) string {
		if keyword == "/kubepods/" {
			return "11:freezer:/kubepods/besteffort/pod23b7d80b-7b31-11e8-9fa1-0ea6a2c824d6/32fd701b15f2a907051d3b07b791cc08d45696c3aa372a4764c98c8be9c57626"
		} else {
			return ""
		}
	}

	id = getContainerIDFromString(mockGetContainerMeta)
	assert.Equal(t, "32fd701b15f2a907051d3b07b791cc08d45696c3aa372a4764c98c8be9c57626", id)

	id = "unintialized"

	var badGetContainerMeta = func(keyword string) string {
		if keyword == "/kubepods/" {
			return "11:freezer:/kubepods/besteffort/pod23b7d80b-7b31-11e8-9fa1-0ea6a2c824d6/abc123hello-world"
		} else {
			return ""
		}
	}

	id = getContainerIDFromString(badGetContainerMeta)
	assert.Equal(t, "", id)
}

func TestGetAWSMetadata(t *testing.T) {
	testEc2MetadataZoneURL := "http://localhost:8880/latest/meta-data/placement/availability-zone"
	testEc2MetadataInstanceIDURL := "http://localhost:8880/latest/meta-data/instance-hostId"

	sm := http.NewServeMux()
	sm.HandleFunc("/latest/meta-data/instance-hostId", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "i-12345678")
	})
	sm.HandleFunc("/latest/meta-data/placement/availability-zone", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "us-east-7")
	})

	addr := "localhost:8880"
	ln, err := net.Listen("tcp", addr)
	require.NoError(t, err)

	s := &http.Server{Addr: addr, Handler: sm}
	// change EC2 MD URLs
	go s.Serve(ln)
	defer func() { // restore old URLs
		ln.Close()
	}()
	time.Sleep(50 * time.Millisecond)

	id := getAWSMeta(testEc2MetadataInstanceIDURL)
	assert.Equal(t, "i-12345678", id)
	assert.Equal(t, "i-12345678", id)
	zone := getAWSMeta(testEc2MetadataZoneURL)
	assert.Equal(t, "us-east-7", zone)
	assert.Equal(t, "us-east-7", zone)
}

func TestGetContainerID(t *testing.T) {
	id := getContainerID()
	if utils.GetLineByKeyword("/proc/self/cgroup", "/docker/") != "" ||
		utils.GetLineByKeyword("/proc/self/cgroup", "/ecs/") != "" {

		assert.NotEmpty(t, id)
		assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]+$`), id)
	} else {
		assert.Empty(t, id)
	}
}
