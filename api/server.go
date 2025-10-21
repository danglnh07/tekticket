package api

import (
	"net/http"
	"tekticket/db"
	"tekticket/service/cloudinary"
	"tekticket/service/security"
	"tekticket/service/worker"

	"github.com/gin-gonic/gin"
)

// Server struct, holds the router, dependencies, system config and logger
type Server struct {
	// API router
	router *gin.Engine

	// Queries
	queries *db.Queries

	// Dependencies
	jwtService    *security.JWTService
	distributor   worker.TaskDistributor
	uploadService *cloudinary.CloudinaryService
}

// Constructor method for server struct
func NewServer(
	queries *db.Queries,
	jwtService *security.JWTService,
	distributor worker.TaskDistributor,
	uploadService *cloudinary.CloudinaryService,
) *Server {
	return &Server{
		router:        gin.Default(),
		queries:       queries,
		jwtService:    jwtService,
		distributor:   distributor,
		uploadService: uploadService,
	}
}

// Helper method to register handler for API
func (server *Server) RegisterHandler() {
	server.router.Use(server.CORSMiddleware())

	// API routes
	api := server.router.Group("/api")
	{
		api.GET("/", func(ctx *gin.Context) {
			ctx.JSON(http.StatusOK, gin.H{"message": "Hello world"})
		})
	}
}

// Start server
func (server *Server) Start() error {
	server.RegisterHandler()
	return server.router.Run(":8080")
}

// Error response struct
type ErrorResponse struct {
	Message string `json:"error"`
}
