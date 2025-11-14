package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
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
