package api

import (
	"net/http"
	"tekticket/db"
	_ "tekticket/docs"
	"tekticket/service/notify"
	"tekticket/service/uploader"
	"tekticket/service/worker"
	"tekticket/util"

	"github.com/gin-gonic/gin"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Server struct, holds the router, dependencies, system config and logger
type Server struct {
	// API router
	router *gin.Engine

	// Queries
	queries *db.Queries

	// Dependencies
	distributor   worker.TaskDistributor
	uploadService *uploader.CloudinaryService
	mailService   notify.MailService

	config *util.Config
}

// Constructor method for server struct
func NewServer(
	queries *db.Queries,
	distributor worker.TaskDistributor,
	mailService notify.MailService,
	uploadService *uploader.CloudinaryService,
	config *util.Config,
) *Server {
	return &Server{
		router:        gin.Default(),
		queries:       queries,
		distributor:   distributor,
		uploadService: uploadService,
		mailService:   mailService,
		config:        config,
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

		auth := api.Group("/auth")
		{
			auth.POST("/register", server.Register)
			auth.POST("/verify/:id", server.VerifyAccount)
			auth.POST("/resend-otp/:id", server.SendOTP)
			auth.POST("/login", server.Login)
			auth.POST("/logout", server.Logout)
		}
	}

	// Swagger docs
	server.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}

// Start server
func (server *Server) Start() error {
	server.RegisterHandler()
	util.LOGGER.Info("Server running. Visit API document at: http://localhost:8080/swagger/index.html")
	return server.router.Run(":8080")
}

// Error response struct
type ErrorResponse struct {
	Message string `json:"error"`
}

// String message
type SuccessMessage struct {
	Message string `json:"message"`
}
