// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package tv_test

import (
	"time"

	"github.com/appneta/go-traceview/v1/tv"
	"golang.org/x/net/context"
)

func slowFunc(ctx context.Context) {
	defer tv.BeginProfile(ctx, "slowFunc").End()
	// ... do something else ...
	time.Sleep(1 * time.Second)
}

func Example() {
	ctx := tv.NewContext(context.Background(), tv.NewTrace("myLayer"))
	slowFunc(ctx)
	tv.EndTrace(ctx)
}
