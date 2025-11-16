package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

// CORS middleware
func (server *Server) CORSMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ctx.Header("Access-Control-Allow-Origin", "*")
		ctx.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		ctx.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")

		// Handle preflight and return immediately so Gin doesn't respond 404 for OPTIONS
		if ctx.Request.Method == http.MethodOptions {
			ctx.AbortWithStatus(http.StatusOK)
			return
		}

		ctx.Next()
	}
}

// Authorization middleware: check if client provided access token for protected API
func (server *Server) AuthMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token := strings.TrimPrefix(ctx.Request.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{"Unauthorized access"})
		}
		ctx.Next()
	}
}

// Prometheus middleware
func (server *Server) PrometheusMiddleware() gin.HandlerFunc {
	// HTTP requests count
	requestCount := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	// HTTP requests duration
	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	// HTTP requests and responses size
	requestSize := prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "http_request_size_bytes",
			Help: "Request size in bytes.",
		},
		[]string{"method", "path"},
	)

	return func(ctx *gin.Context) {
		// Record the start of the request
		start := time.Now()

		// Redirect to the actual API handler
		ctx.Next()

		// Get the request metadata: method, path and status
		method := ctx.Request.Method
		path := ctx.FullPath()
		status := fmt.Sprintf("%d", ctx.Writer.Status())

		// Provide prometheus with metadatas
		requestCount.WithLabelValues(method, path, status).Inc()
		requestDuration.WithLabelValues(method, path, status).Observe(time.Since(start).Seconds())
		requestSize.WithLabelValues(method, path).Observe(time.Since(start).Seconds())
	}
}
