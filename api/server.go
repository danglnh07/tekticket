package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"tekticket/db"
	_ "tekticket/docs"
	"tekticket/service/notify"
	"tekticket/service/uploader"
	"tekticket/service/worker"
	"tekticket/util"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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

		profile := api.Group("/profile")
		{
			profile.GET("", server.GetProfile)
			profile.PUT("", server.UpdateProfile)
		}
	}

	// Swagger docs
	server.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Static handler
	server.router.GET("/images/:id", func(ctx *gin.Context) {
		// Get asset ID
		id := ctx.Param("id")

		// Redirect to the /assets/:id of Directus
		ctx.Redirect(http.StatusPermanentRedirect, "http://localhost:8055/assets/"+id)
	})
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

// Get the Bearer token
func (server *Server) GetToken(ctx *gin.Context) string {
	return strings.TrimPrefix(ctx.Request.Header.Get("Authorization"), "Bearer ")
}

type ImageResponse struct {
	ID string `json:"id"`
}

// Upload image to both Cloudinary and Directus
func (server *Server) UploadImage(ctx *gin.Context, image string) (string, error) {
	path := ctx.FullPath()
	method := ctx.Request.Method

	// Upload the image into cloud service
	cloudResp, err := server.uploadService.UploadImage(ctx, image, uuid.New().String())
	if err != nil {
		util.LOGGER.Error(fmt.Sprintf("%s %s: failed to upload image into the cloud", method, path), "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return "", err
	}

	// Upload the image into Directus
	url := server.config.DirectusAddr + "/files/import"
	directusResp, status, err := util.MakeRequest(
		"POST",
		url,
		map[string]any{"url": cloudResp.SecureURL},
		server.config.DirectusStaticToken,
	)

	if err != nil {
		util.LOGGER.Error(fmt.Sprintf("%s %s: failed to upload image into Directus", method, path), "error", err)
		ctx.JSON(status, ErrorResponse{directusResp.Status})
		return "", err
	}

	// Decode response from Directus
	var diretusResult struct {
		Data ImageResponse `json:"data"`
	}

	if err := json.NewDecoder(directusResp.Body).Decode(&diretusResult); err != nil {
		util.LOGGER.Error(fmt.Sprintf("%s %s: failed to decode directus response", method, path), "error", err)
		ctx.JSON(status, ErrorResponse{directusResp.Status})
		return "", err
	}

	return diretusResult.Data.ID, nil
}
