package api

import (
	"encoding/json"
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
	uploadService *uploader.Uploader
	bot           *bot.Chatbot
	config        *util.Config
}

// Constructor method for server struct
func NewServer(
	queries *db.Queries,
	distributor worker.TaskDistributor,
	mailService notify.MailService,
	uploadService *uploader.Uploader,
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

		// Auth routes
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

		// Profile routes
		profile := api.Group("/profile", server.AuthMiddleware())
		{
			profile.GET("", server.GetProfile)
			profile.PUT("", server.UpdateProfile)
		}

		// Booking routes
		booking := api.Group("/bookings", server.AuthMiddleware())
		{
			booking.GET("", server.ListBookingHistory)
			booking.GET("/:id", server.GetBooking)
			booking.POST("", server.CreateBooking)
		}

		// Payment routes
		payments := api.Group("/payments", server.AuthMiddleware())
		{
			payments.POST("", server.CreatePayment)
			payments.GET("/method", server.CreatePaymentMethod)
			payments.POST("/:id/confirm", server.ConfirmPayment)
			payments.POST("/:id/refund", server.Refund)
			payments.POST("/:id/retry-qr-publishing", server.RetryQRPublishing)
		}

		// Checkin routes
		checkin := api.Group("/checkins")
		{
			checkin.POST("", server.Checkin)
		}

		// Categories routes
		categories := api.Group("/categories", server.AuthMiddleware())
		{
			categories.GET("", server.GetCategories)
		}

		// Event routes
		events := api.Group("/events", server.AuthMiddleware())
		{
			events.GET("", server.ListEvents)
			events.GET("/:id", server.GetEvent)
		}

		// Memberships routes
		memberships := api.Group("/memberships", server.AuthMiddleware())
		{
			memberships.GET("", server.ListMemberships)
			memberships.GET("/me", server.GetUserMembership)
		}

		// Webhook handler
		webhook := api.Group("/webhook")
		{
			webhook.POST("/telegram", server.TelegramWebhook)
		}

		// Notification
		notification := api.Group("/notifications")
		{
			notification.POST("/webhook", server.NotificationWebhook)
		}
	}

	// Swagger docs
	server.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Static image handler
	server.router.GET("/images/:id", server.GetImage)
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

// Success message struct
type SuccessMessage struct {
	Message string `json:"message"`
}

// Image handler
func (server *Server) GetImage(ctx *gin.Context) {
	id := ctx.Param("id")

	// Since we need the Response object for redirecting, so we'll manually make request here, not using the db.MakeRequest method
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
		var errMsg db.DirectusErrorResp
		if err := json.NewDecoder(resp.Body).Decode(&errMsg); err != nil {
			util.LOGGER.Error("GET /images/:id: failed to read error messages", "error", err)
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"failed to read error message"})
			return
		}
		util.LOGGER.Error("GET /images:id: Directus error message", "err", errMsg)
		ctx.JSON(resp.StatusCode, ErrorResponse{})
		return
	}

	ctx.Header("Content-Type", resp.Header.Get("Content-Type"))
	io.Copy(ctx.Writer, resp.Body)
}

// Helper method: get access token from Authorization header
func (server *Server) GetToken(ctx *gin.Context) string {
	return strings.TrimPrefix(ctx.Request.Header.Get("Authorization"), "Bearer ")
}

// Helper method: handling directus error
func (server *Server) DirectusError(ctx *gin.Context, err error) {
	if db.IsDirectusError(err) {
		directusErr := err.(*db.DirectusErrorResp).Errors[0]
		code := directusErr.Extension.Code
		message := directusErr.Message

		switch code {
		case db.FAILED_VALIDATION:
			// For failed validation, although the server side can also make such mistake, but this error should be client side error
			msg := fmt.Sprintf("Invalid request data: %s", message)
			ctx.JSON(http.StatusBadRequest, ErrorResponse{msg})
		case db.FORBIDDEN:
			// Forbidden is the trickiest one here. In Directus, a FORBIDDEN request can be:
			// 1. You don't access permission to that collections/fields
			// 2. You access into a field name that is not exists (typo, for example, 'statu' instead of 'status')
			// 3. You access an item with none existing ID. Normally, this should be 404 status code, but Directus return 403 to
			// prevent revealing which items exist, according to their docs.
			// Because of that, for this status code, we'll assume this to be client side, and return a 404 code
			// (for the first and second cases, such mistakes can be prevent for some simple testing, so we'll only check the third case)
			ctx.JSON(http.StatusNotFound, ErrorResponse{"No item with such ID"})
		case db.INVALID_TOKEN:
			// Token invalid. Most of operation use the client access token, only some require admin static token,
			// so we can assume this is client fault
			ctx.JSON(http.StatusForbidden, ErrorResponse{"Invalid token"})
		case db.TOKEN_EXPIRED:
			// Obviously, client side error
			ctx.JSON(http.StatusUnauthorized, ErrorResponse{"token expired"})
		case db.INVALID_CREDENTIALS:
			// This should be for login. Obviously, client side error
			ctx.JSON(http.StatusUnauthorized, ErrorResponse{"incorrect login credentials"})
		case db.INVALID_IP:
			// You can setup CORS for Directus, which allow a set of IPs. Normally, only our server can reach Directus,
			// so this should be server side if server IP is not allow in Directus
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		case db.INVALID_PAYLOAD:
			// Invalid payload request. This should be server side error most of the time, since it's the server who make request
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		case db.INVALID_QUERY:
			// Invalid query string in URL. Server side error
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		case db.REQUESTS_EXCEEDED:
			// You hit the rate limit of Directus. Although server side can also make such mistakes, it would mostly client
			// who spam the APIs
			ctx.JSON(http.StatusTooManyRequests, ErrorResponse{"You hit the rate limit"})
		case db.ROUTE_NOT_FOUND:
			// Since it's server who make requests, server side error
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		default:
			// For other code that didn't get listed:
			// 1. INVALID_OTP: only happen we using Directus OTP functionality. We use our own OTP validation, so this should never
			// happen
			// 2. UNSUPPORTED_MEDIA_TYPE: mostly never happen
			// 3. SERVICE_UNAVAILABLE: currently Directus didn't interact with external service
			// 4. UNPROCESSABLE_CONTENT: server side is the one control the final data to be sent to Directus, so this should never
			// happen
			// But for reliability, we'll also return a 500 error
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		}
	} else {
		// If not Directus error -> server side error
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
	}
}
