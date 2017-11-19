// Copyright (C) 2017 Librato, Inc. All rights reserved.

package ottv

import (
	"bytes"
	"fmt"
	"io/ioutil"
	golog "log"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	g "github.com/librato/go-traceview/v1/tv/internal/graphtest"
	"github.com/librato/go-traceview/v1/tv/internal/traceview"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

// client/server test based on basictracer-go/examples/dapperish.go

const (
	testTextKey    = "user text"
	testTextVal    = "LogFields user text value"
	testBaggageKey = "user"
	testBaggageVal = "BaggageUser"
)

func client(t *testing.T, port int, wg *sync.WaitGroup) {
	span := opentracing.StartSpan("getInput")
	ctx := opentracing.ContextWithSpan(context.Background(), span)
	// Make sure that global baggage propagation works.
	span.SetBaggageItem(testBaggageKey, testBaggageVal)
	span.LogFields(log.Object("ctx", ctx))
	text := strings.TrimSpace(testTextVal)
	span.LogFields(log.String(testTextKey, text))

	httpClient := &http.Client{}
	httpReq, _ := http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/", port), bytes.NewReader([]byte(text)))
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
	wg.Done() // signal to TestReporter that reporting is done
}

func server(t *testing.T, list net.Listener) {
	s := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
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
	})}

	golog.Fatal(s.Serve(list))
}

func TestTracer(t *testing.T) {
	r := traceview.SetTestReporter() // set up test reporter
	opentracing.InitGlobalTracer(NewTracer())

	ln, err := net.Listen("tcp", ":0") // pick an unallocated port
	assert.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port

	var wg sync.WaitGroup
	go server(t, ln)
	wg.Add(1)
	go client(t, port, &wg)

	wg.Wait()
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
