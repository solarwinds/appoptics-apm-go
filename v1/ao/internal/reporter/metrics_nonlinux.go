// +build !linux

// Copyright (C) 2017 Librato, Inc. All rights reserved.

package reporter

func appendUname(bbuf *bsonBuffer) {}

func addHostMetrics(bbuf *bsonBuffer, index *int) {}
