// Copyright (C) 2016 Librato, Inc. All rights reserved.

package ao_test

import (
	"os"
	"strings"
	"testing"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
	g "github.com/appoptics/appoptics-apm-go/v1/ao/internal/graphtest"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/reporter"
	"github.com/stretchr/testify/assert"
)

func TestCustomTransactionNameWithDomain(t *testing.T) {
	os.Setenv("APPOPTICS_PREPEND_DOMAIN", "true")
	config.Load()
	r := reporter.SetTestReporter() // set up test reporter

	// Test prepending the domain to transaction names.
	httpTestWithEndpoint(handler200CustomTxnName, "http://test.com/hello world/one/two/three?testq")
	r.Close(2)
	g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeMap{
		// entry event should have no edges
		{"http.HandlerFunc", "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
			assert.Equal(t, "test.com", n.Map["HTTP-Host"])
		}},
		{"http.HandlerFunc", "exit"}: {Edges: g.Edges{{"http.HandlerFunc", "entry"}}, Callback: func(n g.Node) {
			// assert that response X-Trace header matches trace exit event
			assert.True(t, strings.HasPrefix(n.Map["TransactionName"].(string),
				"test.com/final-my-custom-transaction-name"),
				n.Map["TransactionName"].(string))
		}},
	})

	r = reporter.SetTestReporter() // set up test reporter

	// Test using X-Forwarded-Host if available.
	hd := map[string]string{
		"X-Forwarded-Host": "test2.com",
	}
	httpTestWithEndpointWithHeaders(handler200CustomTxnName, "http://test.com/hello world/one/two/three?testq", hd)
	r.Close(2)
	g.AssertGraph(t, r.EventBufs, 2, g.AssertNodeMap{
		// entry event should have no edges
		{"http.HandlerFunc", "entry"}: {Edges: g.Edges{}, Callback: func(n g.Node) {
			assert.Equal(t, "test.com", n.Map["HTTP-Host"])
		}},
		{"http.HandlerFunc", "exit"}: {Edges: g.Edges{{"http.HandlerFunc", "entry"}}, Callback: func(n g.Node) {
			// assert that response X-Trace header matches trace exit event
			assert.True(t, strings.HasPrefix(n.Map["TransactionName"].(string),
				"test2.com/final-my-custom-transaction-name"),
				n.Map["TransactionName"].(string))
		}},
	})
	os.Unsetenv("APPOPTICS_PREPEND_DOMAIN")
}
