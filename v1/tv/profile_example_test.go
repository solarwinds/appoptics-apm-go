// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package tv_test

import (
	"github.com/appneta/go-traceview/v1/tv"
	"golang.org/x/net/context"
)

func ExampleBeginProfile(ctx context.Context) {
	defer tv.BeginProfile(ctx, "example").End()
	// ... do something ...
}

func ExampleBeginProfile_func(ctx context.Context) {
	// typically this would be used in a named function
	func() {
		defer tv.BeginProfile(ctx, "example_func").End()
		// ... do something else ...
	}()
}
