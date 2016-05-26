package main_test

import (
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient(t *testing.T) {
	// run distributed_app server

	// test request to distributed_app example
	httpClient := &http.Client{}
	resp, err := httpClient.Get("http://127.0.0.1:8890/lookup")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)
	t.Logf("httpClient.Do body: %v, err: %v", string(body), err)
}
