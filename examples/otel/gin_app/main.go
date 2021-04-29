package main

import (
	"net/http"

	ao "github.com/appoptics/appoptics-apm-go/v1/ao/opentelemetry"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
)

func main() {
	tp, _ := ao.NewTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(ao.AOTraceContext{})

	// Creates a gin router with default middleware:
	// logger and recovery (crash-free) middleware
	router := gin.Default()

	// add AppOptics middleware
	router.Use(otelgin.Middleware("my-server"))

	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Hello from Gin")
	})

	// By default it serves on :8080 unless a
	// PORT environment variable was defined.
	router.Run()
}
