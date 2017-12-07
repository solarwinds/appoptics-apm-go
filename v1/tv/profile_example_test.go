// Copyright (C) 2016 Librato, Inc. All rights reserved.

package tv_test

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/librato/go-traceview/v1/tv"
	"golang.org/x/net/context"
)

// measure a DB query
func dbQuery(ctx context.Context, host, query string, args ...interface{}) *sql.Rows {
	// Begin a AppOptics span for this DB query
	l, _ := tv.BeginSpan(ctx, "dbQuery", "Query", query, "RemoteHost", host)
	defer l.End()

	db, err := sql.Open("mysql", fmt.Sprintf("user:password@tcp(%s:3306)/db", host))
	if err != nil {
		l.Err(err) // Report error & stack trace on Span
		return nil
	}
	defer db.Close()
	rows, err := db.Query(query, args...)
	if err != nil {
		l.Err(err)
	}
	return rows
}

// measure a slow function
func slowFunc(ctx context.Context) {
	defer tv.BeginProfile(ctx, "slowFunc").End()
	time.Sleep(1 * time.Second)
}

func Example() {
	ctx := tv.NewContext(context.Background(), tv.NewTrace("mySpan"))
	_ = dbQuery(ctx, "dbhost.net", "SELECT * from tbl LIMIT 1")
	slowFunc(ctx)
	tv.EndTrace(ctx)
}
