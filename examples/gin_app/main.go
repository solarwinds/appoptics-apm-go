package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func main() {
	// Creates a gin router with default middleware:
	// logger and recovery (crash-free) middleware
	router := gin.Default()

	// add TraceView middleware
	router.Use(Tracer())

	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Hello from Gin")
	})

	// By default it serves on :8080 unless a
	// PORT environment variable was defined.
	router.Run()
}
