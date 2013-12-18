package traceview

import (
	"testing"
)

var test_layer string = "go_test"

func TestSendEvent(t *testing.T) {
	ctx := NewContext()
	e := ctx.NewEvent(LabelEntry, test_layer)
	e.AddInt("IntTest", 123)

	if err := e.Report(ctx); err != nil {
		t.Errorf("Unexpected %v", err)
	}

	e = ctx.NewEvent(LabelExit, test_layer)
	e.AddEdge(ctx)
	if err := e.Report(ctx); err != nil {
		t.Errorf("Unexpected %v", err)
	}

}
