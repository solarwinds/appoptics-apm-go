// Copyright (C) 2017 Librato, Inc. All rights reserved.

package ottv

import (
	"bytes"
	"io/ioutil"
	golog "log"
	"net/http"
	"strings"
	"testing"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	g "github.com/tracelytics/go-traceview/v1/tv/internal/graphtest"
	"github.com/tracelytics/go-traceview/v1/tv/internal/traceview"
	"golang.org/x/net/context"
)

// client/server test based on basictracer-go/examples/dapperish.go

const (
	testTextKey    = "user text"
	testTextVal    = "LogFields user text value"
	testBaggageKey = "user"
	testBaggageVal = "BaggageUser"
)

func client(t *testing.T) {
	span := opentracing.StartSpan("getInput")
	ctx := opentracing.ContextWithSpan(context.Background(), span)
	// Make sure that global baggage propagation works.
	span.SetBaggageItem(testBaggageKey, testBaggageVal)
	span.LogFields(log.Object("ctx", ctx))
	text := strings.TrimSpace(testTextVal)
	span.LogFields(log.String(testTextKey, text))

	httpClient := &http.Client{}
	httpReq, _ := http.NewRequest("POST", "http://localhost:8080/", bytes.NewReader([]byte(text)))
	textCarrier := opentracing.HTTPHeadersCarrier(httpReq.Header)
	err := span.Tracer().Inject(span.Context(), opentracing.TextMap, textCarrier)
	require.NoError(t, err)

	resp, err := httpClient.Do(httpReq)
	assert.NoError(t, err)
	if err != nil {
		span.LogFields(log.Error(err))
	} else {
		span.LogFields(log.Object("response", resp))
	}

	span.Finish()
}

func server(t *testing.T) {
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		textCarrier := opentracing.HTTPHeadersCarrier(req.Header)
		wireSpanContext, err := opentracing.GlobalTracer().Extract(
			opentracing.TextMap, textCarrier)
		require.NoError(t, err)

		serverSpan := opentracing.GlobalTracer().StartSpan(
			"serverSpan",
			ext.RPCServerOption(wireSpanContext))
		serverSpan.SetTag("component", "server")
		ext.HTTPUrl.Set(serverSpan, req.URL.String())
		ext.HTTPMethod.Set(serverSpan, req.Method)
		defer serverSpan.Finish()
		// check baggage propagation
		assert.Equal(t, serverSpan.BaggageItem(testBaggageKey), testBaggageVal)

		fullBody, err := ioutil.ReadAll(req.Body)
		assert.NoError(t, err)
		if err != nil {
			serverSpan.LogFields(log.Error(err))
		}
		serverSpan.LogFields(log.String("request body", string(fullBody)))
	})

	golog.Fatal(http.ListenAndServe(":8080", nil))
}

func TestTracer(t *testing.T) {
	r := traceview.SetTestReporter() // set up test reporter
	opentracing.InitGlobalTracer(NewTracer())

	go server(t)
	go client(t)

	r.Close(4)
	g.AssertGraph(t, r.Bufs, 4, g.AssertNodeMap{
		{"getInput", "entry"}: {},
		{"getInput", "exit"}: {Edges: g.Edges{{"getInput", "entry"}}, Callback: func(n g.Node) {
			assert.Equal(t, testTextVal, n.Map[otLogPrefix+testTextKey])
		}},
		{"serverSpan", "entry"}: {Edges: g.Edges{{"getInput", "entry"}}, Callback: nil},
		{"serverSpan", "exit"}: {Edges: g.Edges{{"serverSpan", "entry"}}, Callback: func(n g.Node) {
			assert.Equal(t, "server", n.Map["OTComponent"])
			assert.Equal(t, "/", n.Map["URL"])
			assert.Equal(t, "POST", n.Map["Method"])
			assert.Equal(t, testTextVal, n.Map[otLogPrefix+"request body"])
		}},
	})
}
