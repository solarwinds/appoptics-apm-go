// +build !linux

// Copyright (C) 2017 Librato, Inc. All rights reserved.

package reporter

import "github.com/appoptics/appoptics-apm-go/v1/ao/internal/bson"

func appendUname(bbuf *bson.Buffer) {}

func addHostMetrics(bbuf *bson.Buffer, index *int) {}
