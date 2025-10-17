package api

import (
	"net/http"
	"sync"
	"tekticket/db"
	"tekticket/service/security"
	"tekticket/service/worker"
	"tekticket/util"

	"github.com/gin-gonic/gin"
)

// Server struct, holds the router, dependencies, system config and logger
type Server struct {
	// API router
	router *gin.Engine

	// Queries
	queries *db.Queries

	// Dependencies
	jwtService  *security.JWTService
	distributor worker.TaskDistributor

	// In-memory OTP store (identifier -> otp)
	otpMu    sync.RWMutex
	otpStore map[string]string
}

// Constructor method for server struct
func NewServer(
	queries *db.Queries,
	jwtService *security.JWTService,
	distributor worker.TaskDistributor,
) *Server {
	return &Server{
		router:      gin.Default(),
		queries:     queries,
		jwtService:  jwtService,
		distributor: distributor,
		otpStore:    map[string]string{},
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
			auth.POST("/register", server.handleRegister)
			auth.POST("/verify", server.handleVerify)
			auth.POST("/login", server.handleLogin)
			auth.POST("/refresh", server.handleRefresh)
		}

		// User routes (protected)
		users := api.Group("/users")
		users.Use(server.AuthMiddleware())
		{
			users.GET("/me", server.handleGetMe)
			users.PUT("/me", server.handleUpdateMe)
		}
	}
}

// ----- Auth Handlers -----

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	Username string `json:"username"`
	Phone    string `json:"phone"`
}

func (server *Server) handleRegister(ctx *gin.Context) {
	var req registerRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid payload"})
		return
	}

	// Defaults
	username := req.Username
	if username == "" {
		username = req.Email
	}
	phone := req.Phone
	if phone == "" {
		phone = ""
	}
	avatar := "https://www.gravatar.com/avatar/default.png"

	// Hash password
	hashed, err := security.BcryptHash(req.Password)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{Message: "failed to hash password"})
		return
	}

	user := &db.User{
		Username:     username,
		Email:        req.Email,
		Phone:        phone,
		Password:     hashed,
		Avatar:       avatar,
		Role:         "customer",
		Status:       "inactive",
		TokenVersion: 0,
	}

	if err := server.queries.CreateUser(ctx, user); err != nil {
		ctx.JSON(http.StatusConflict, ErrorResponse{Message: "email/username/phone already exists"})
		return
	}

	// Generate OTP and store in-memory keyed by email
	server.otpMu.Lock()
	server.otpStore[user.Email] = util.RandomString(6)
	otp := server.otpStore[user.Email]
	server.otpMu.Unlock()

	// Mock send OTP (log only)
	util.LOGGER.Info("Mock send OTP", "email", user.Email, "otp", otp)

	ctx.JSON(http.StatusCreated, gin.H{"message": "registered, please verify", "email": user.Email})
}

type verifyRequest struct {
	Email string `json:"email"`
	Phone string `json:"phone"`
	OTP   string `json:"otp" binding:"required"`
}

func (server *Server) handleVerify(ctx *gin.Context) {
	var req verifyRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid payload"})
		return
	}

	key := req.Email
	if key == "" {
		key = req.Phone
	}
	if key == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{Message: "email or phone required"})
		return
	}

	server.otpMu.RLock()
	otp, ok := server.otpStore[key]
	server.otpMu.RUnlock()
	if !ok || otp != req.OTP {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid otp"})
		return
	}

	// Activate user
	var user *db.User
	var err error
	if req.Email != "" {
		user, err = server.queries.GetUserByEmail(ctx, req.Email)
	}
	if err != nil || user == nil {
		ctx.JSON(http.StatusNotFound, ErrorResponse{Message: "user not found"})
		return
	}
	user.Status = "active"
	if err := server.queries.UpdateUser(ctx, user); err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{Message: "failed to update user"})
		return
	}

	// Clear OTP
	server.otpMu.Lock()
	delete(server.otpStore, key)
	server.otpMu.Unlock()

	ctx.JSON(http.StatusOK, gin.H{"message": "verified"})
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (server *Server) handleLogin(ctx *gin.Context) {
	var req loginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid payload"})
		return
	}

	user, err := server.queries.GetUserByEmail(ctx, req.Email)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{Message: "invalid credentials"})
		return
	}
	if !security.BcryptCompare(user.Password, req.Password) {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{Message: "invalid credentials"})
		return
	}
	if user.Status != "active" {
		ctx.JSON(http.StatusForbidden, ErrorResponse{Message: "account not verified"})
		return
	}

	access, err := server.jwtService.CreateToken(user.ID, security.AccessToken, user.TokenVersion)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{Message: "failed to create token"})
		return
	}
	refresh, err := server.jwtService.CreateToken(user.ID, security.RefreshToken, user.TokenVersion)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{Message: "failed to create token"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"access_token": access, "refresh_token": refresh})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (server *Server) handleRefresh(ctx *gin.Context) {
	var req refreshRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid payload"})
		return
	}
	claims, err := server.jwtService.VerifyToken(req.RefreshToken)
	if err != nil || claims.TokenType != security.RefreshToken {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{Message: "invalid refresh token"})
		return
	}
	user, err := server.queries.GetUserByID(ctx, claims.ID)
	if err != nil || user.TokenVersion != claims.Version {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{Message: "refresh revoked"})
		return
	}
	access, err := server.jwtService.CreateToken(user.ID, security.AccessToken, user.TokenVersion)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{Message: "failed to create token"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"access_token": access})
}

// ----- User Handlers -----

func (server *Server) handleGetMe(ctx *gin.Context) {
	user, _ := ctx.Get("currentUser")
	ctx.JSON(http.StatusOK, user)
}

type updateMeRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Avatar   string `json:"avatar"`
}

func (server *Server) handleUpdateMe(ctx *gin.Context) {
	cur, _ := ctx.Get("currentUser")
	user := cur.(*db.User)

	var req updateMeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{Message: "invalid payload"})
		return
	}

	// If email/phone changes, require OTP verification first (mock: generate OTP and return a message)
	if (req.Email != "" && req.Email != user.Email) || (req.Phone != "" && req.Phone != user.Phone) {
		// generate OTP for new identifier
		identifier := req.Email
		if identifier == "" {
			identifier = req.Phone
		}
		if identifier == "" {
			ctx.JSON(http.StatusBadRequest, ErrorResponse{Message: "email or phone required"})
			return
		}
		server.otpMu.Lock()
		server.otpStore[identifier] = util.RandomString(6)
		otp := server.otpStore[identifier]
		server.otpMu.Unlock()
		util.LOGGER.Info("Mock send OTP for update", "id", identifier, "otp", otp)
		ctx.JSON(http.StatusAccepted, gin.H{"message": "verify new contact with OTP", "identifier": identifier})
		return
	}

	if req.Username != "" {
		user.Username = req.Username
	}
	if req.Avatar != "" {
		user.Avatar = req.Avatar
	}
	if err := server.queries.UpdateUser(ctx, user); err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{Message: "failed to update"})
		return
	}
	ctx.JSON(http.StatusOK, user)
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
