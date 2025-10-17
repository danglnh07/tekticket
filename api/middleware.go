package api

import (
	"net/http"
	"strings"
	"tekticket/service/security"
	"tekticket/util"

	"github.com/gin-gonic/gin"
)

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

// AuthMiddleware validates JWT access token and loads current user
func (server *Server) AuthMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		authHeader := ctx.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Message: "missing bearer token"})
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := server.jwtService.VerifyToken(token)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Message: "invalid token"})
			return
		}
		if claims.TokenType != security.AccessToken {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Message: "invalid token type"})
			return
		}

		// Load user and compare token version
		user, err := server.queries.GetUserByID(ctx, claims.ID)
		if err != nil {
			util.LOGGER.Error("Auth load user failed", "error", err)
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Message: "unauthorized"})
			return
		}
		if user.TokenVersion != claims.Version {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Message: "token revoked"})
			return
		}

		// Store to context
		ctx.Set("currentUser", user)
		ctx.Next()
	}
}
