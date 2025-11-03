package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"tekticket/db"
	_ "tekticket/docs"
	"tekticket/service/bot"
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
	mailService   notify.MailService
	uploadService *uploader.CloudinaryService
	bot           *bot.Chatbot
	config        *util.Config
}

// Constructor method for server struct
func NewServer(
	queries *db.Queries,
	distributor worker.TaskDistributor,
	mailService notify.MailService,
	uploadService *uploader.CloudinaryService,
	bot *bot.Chatbot,
	config *util.Config,
) *Server {
	return &Server{
		router:        gin.Default(),
		queries:       queries,
		distributor:   distributor,
		uploadService: uploadService,
		mailService:   mailService,
		bot:           bot,
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
			auth.POST("/refresh", server.RefreshToken)
			auth.POST("/password/request", server.SendResetPasswordRequest)
			auth.POST("/password/reset", server.ResetPassword)
		}

		profile := api.Group("/profile")
		{
			profile.GET("", server.GetProfile)
			profile.PUT("", server.UpdateProfile)
		}

		categories := api.Group("/categories")
		{
			categories.GET("", server.GetCategories)
		}

		events := api.Group("/events")
		{
			events.GET("", server.ListEvents)
			events.GET("/:id", server.GetEvent)
		}

		// Memberships routes
		memberships := api.Group("/memberships")
		{
			memberships.GET("", server.ListMemberships)
			memberships.GET("/:id", server.GetUserMembership)
		}

		// Bookings routes
		bookings := api.Group("/bookings")
		{
			bookings.POST("", server.CreateBooking)
			bookings.POST("/checkout", server.Checkout)
		}

		// Webhook handler
		webhook := api.Group("/webhook")
		{
			webhook.POST("/telegram", server.TelegramWebhook)
		}
	}

	// Swagger docs
	server.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Static handler
	server.router.GET("/images/:id", func(ctx *gin.Context) {
		id := ctx.Param("id")

		// Since we need the Response object for redirecting, so we'll manually make request here, not using the util.MakeRequest
		url := fmt.Sprintf("%s/assets/%s", server.config.DirectusAddr, id)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			util.LOGGER.Error("GET /images/:id: failed to create request", "error", err)
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
			return

		}
		req.Header.Set("Authorization", "Bearer "+server.config.DirectusStaticToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			util.LOGGER.Error("GET /images/:id: failed to get assets", "error", err)
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{err.Error()})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			ctx.JSON(resp.StatusCode, ErrorResponse{resp.Status})
			return
		}

		ctx.Header("Content-Type", resp.Header.Get("Content-Type"))
		io.Copy(ctx.Writer, resp.Body)
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
	var imageResp ImageResponse
	status, err := util.MakeRequest(
		"POST",
		url,
		map[string]any{"url": cloudResp.SecureURL},
		server.config.DirectusStaticToken,
		&imageResp,
	)

	if err != nil {
		util.LOGGER.Error(fmt.Sprintf("%s %s: failed to upload image into Directus", method, path), "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return "", err
	}

	return imageResp.ID, nil
}
