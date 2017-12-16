package main

import (
	"bufio"
	"net"

	"github.com/gin-gonic/gin"
	"github.com/librato/go-traceview/v1/tv"
	"golang.org/x/net/context"
)

const (
	ginContextKey = "AppOptics"
	ginLayerName  = "gin"
)

func Tracer() gin.HandlerFunc {
	return func(c *gin.Context) {
		t, w := tv.TraceFromHTTPRequestResponse("gin", c.Writer, c.Request)
		c.Writer = &ginResponseWriter{w.(*tv.HTTPResponseWriter), c.Writer}
		t.SetControllerAction(ginLayerName, c.HandlerName())
		defer t.End()
		// create a context.Context and bind it to the gin.Context
		c.Set(ginContextKey, tv.NewContext(context.Background(), t))
		// Pass to the next handler
		c.Next()
	}
}

// ginResponseWriter satisfies the gin.ResponseWriter interface
type ginResponseWriter struct {
	// handles Write, WriteHeader, Header (by calling wrapped gin writer)
	*tv.HTTPResponseWriter
	// handles all other gin.ResponseWriter methods
	ginWriter gin.ResponseWriter
}

func (w *ginResponseWriter) CloseNotify() <-chan bool                     { return w.ginWriter.CloseNotify() }
func (w *ginResponseWriter) Flush()                                       { w.ginWriter.Flush() }
func (w *ginResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) { return w.ginWriter.Hijack() }
func (w *ginResponseWriter) Size() int                                    { return w.ginWriter.Size() }
func (w *ginResponseWriter) Written() bool                                { return w.ginWriter.Written() }
func (w *ginResponseWriter) WriteString(s string) (int, error)            { return w.ginWriter.WriteString(s) }
func (w *ginResponseWriter) Status() int                                  { return w.StatusCode }
func (w *ginResponseWriter) WriteHeaderNow() {
	if !w.WroteHeader {
		w.WriteHeader(w.StatusCode)
	}
}
