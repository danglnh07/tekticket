package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"tekticket/db"
	"tekticket/service/worker"
	"tekticket/util"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
)

type RegisterRequest struct {
	Firstname string `json:"firstname" binding:"required"`
	Lastname  string `json:"lastname" binding:"required"`
	Email     string `json:"email" binding:"required,email"`
	Password  string `json:"password" binding:"required"`
	Role      string `json:"role" binding:"required"`
}

type RegisterResponse struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Status    string `json:"status"`
	Role      string `json:"role"`
}

// Register godoc
// @Summary      Register a new user account
// @Description  Creates a new user account with the provided username, email, phone, password, and role.
// @Description  Sends a verification email to activate the account.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request  body      RegisterRequest  true  "Registration request body"
// @Success      201  {object}  RegisterResponse "Create account success with status inactive"
// @Failure      400  {object}  ErrorResponse  "Invalid request body or existing username/email/phone"
// @Failure      500  {object}  ErrorResponse  "Internal server error or failed to send verification email"
// @Router       /api/auth/register [post]
func (server *Server) Register(ctx *gin.Context) {
	// Get request body and validate
	var req RegisterRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// Check roles
	roles, err := server.queries.Client.Roles.List(ctx)
	if err != nil {
		util.LOGGER.Error("POST /api/auth/register: failed to get list of roles", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	roleID := ""
	for _, role := range roles {
		if strings.EqualFold(role.Name, req.Role) {
			roleID = role.ID
		}
	}

	if roleID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid role"})
		return
	}

	// Check if this email has been register
	users, err := server.queries.Client.Users.List(ctx)
	if err != nil {
		util.LOGGER.Error("POST /api/auth/register: failed to get list of users", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	for _, user := range users {
		// Only same email for different role
		if user.Email == req.Email && (roleID == user.Role || strings.EqualFold(user.Role, req.Role)) {
			ctx.JSON(http.StatusBadRequest, ErrorResponse{"This email already registered"})
			return
		}
	}

	// Make request to directus server
	body := map[string]any{
		"first_name": req.Firstname,
		"last_name":  req.Lastname,
		"email":      req.Email,
		"password":   req.Password,
		"role":       roleID,
		"status":     "unverified",
	}

	url := fmt.Sprintf("%s/%s", server.config.DirectusAddr, "users")
	resp, status, err := util.MakeRequest("POST", url, body, server.config.DirectusStaticToken)
	if err != nil {
		util.LOGGER.Error("POST /api/auth/register: failed to make API request to Directus", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	// Parse response
	var directusResp struct {
		Data RegisterResponse `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
		util.LOGGER.Error("POST /api/auth/register: failed to decode Directus response", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	// Create background task: send verify email
	err = server.distributor.DistributeTask(ctx, worker.SendVerifyEmail, worker.SendVerifyEmailPayload{
		ID:       directusResp.Data.ID,
		Email:    directusResp.Data.Email,
		Username: fmt.Sprintf("%s %s", directusResp.Data.FirstName, directusResp.Data.LastName),
	}, asynq.MaxRetry(5))

	if err != nil {
		util.LOGGER.Error("POST /api/auth/register: failed to distribute task", "task", worker.SendVerifyEmail, "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	// return result back to client
	ctx.JSON(http.StatusOK, directusResp.Data)
}

// VerifyAccount godoc
// @Summary      Verify user account
// @Description  Verifies a user's account using the provided OTP code sent to their email.
// @Description  Once verified, the user's account status is set to active.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "User ID"
// @Param        otp  query     string  true  "6-digit OTP verification code"
// @Success      200  {object}  SuccessMessage  "Verify account successfully, please login"
// @Failure      400  {object}  ErrorResponse   "Invalid OTP, expired code, or ID mismatch"
// @Failure      500  {object}  ErrorResponse   "Internal server error"
// @Router       /api/auth/verify/{id} [post]
func (server *Server) VerifyAccount(ctx *gin.Context) {
	// Get user ID and OTP code
	id := ctx.Param("id")
	otp := ctx.Query("otp")

	// OTP validation
	if otp = strings.TrimSpace(otp); otp == "" || len(otp) != 6 {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid OTP code"})
		return
	}

	// Get the from cache
	idCached, err := server.queries.GetCache(ctx, otp)
	if err != nil && errors.Is(err, &db.ErrorCacheMiss{}) {
		util.LOGGER.Error("POST /api/auth/verify: failed to get OTP code from cache", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	if idCached == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"OTP expired"})
		return
	}

	if idCached != id {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"ID mismatch with OTP"})
		return
	}

	// Update status
	url := fmt.Sprintf("%s/%s/%s", server.config.DirectusAddr, "users", id)
	_, status, err := util.MakeRequest("PATCH", url, map[string]any{"status": "active"}, server.config.DirectusStaticToken)
	if err != nil {
		util.LOGGER.Error("POST /api/auth/verify: failed to update account status", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, SuccessMessage{"Validate success"})
}

// ResendOTP godoc
// @Summary      Resend account verification OTP
// @Description  Resends a new OTP code to the user's registered email address if the account is still inactive.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "User ID"
// @Success      200  {object}  SuccessMessage  "OTP resent successfully"
// @Failure      400  {object}  ErrorResponse   "Invalid ID or account not inactive"
// @Failure      500  {object}  ErrorResponse   "Internal server error"
// @Router       /api/auth/resend-otp/{id} [post]
func (server *Server) SendOTP(ctx *gin.Context) {
	// Get ID from path parameter
	id := ctx.Param("id")

	// Check if this user exists
	url := server.config.DirectusAddr + "/users/" + id
	resp, status, err := util.MakeRequest("GET", url, nil, server.config.DirectusStaticToken)
	if err != nil {
		util.LOGGER.Error("POST /api/auth/resend-otp: failed to get user data", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	var directusResp struct {
		Data RegisterResponse `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
		util.LOGGER.Error("POST /api/auth/resend-otp: failed to decode Directus response", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Create background job, send OTP
	err = server.distributor.DistributeTask(ctx, worker.SendVerifyEmail, worker.SendVerifyEmailPayload{
		ID:       id,
		Email:    directusResp.Data.Email,
		Username: fmt.Sprintf("%s %s", directusResp.Data.FirstName, directusResp.Data.LastName),
	}, asynq.MaxRetry(5))

	if err != nil {
		util.LOGGER.Error("POST /api/auth/resend-otp: failed to distribute task", "task", worker.SendVerifyEmail, "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	ctx.JSON(http.StatusOK, SuccessMessage{"OTP resend successfully"})
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Expires      int    `json:"expires"`
}

// Login godoc
// @Summary      User login
// @Description  Authenticate user with username and password. Returns access and refresh JWT tokens.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request  body      LoginRequest  true  "Login request body (username, password)"
// @Success      200  {object}  LoginResponse  "Successful login with access and refresh tokens"
// @Failure      400  {object}  ErrorResponse  "Invalid request body or incorrect credentials"
// @Failure      403  {object}  ErrorResponse  "Account not active, cannot login"
// @Failure      500  {object}  ErrorResponse  "Internal server error"
// @Router       /api/auth/login [post]
func (server *Server) Login(ctx *gin.Context) {
	// Get request body
	var req LoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// Call login request to Directus
	url := fmt.Sprintf("%s/%s/%s", server.config.DirectusAddr, "auth", "login")
	resp, status, err := util.MakeRequest("POST", url, map[string]any{
		"email":    req.Email,
		"password": req.Password,
	}, server.config.DirectusStaticToken)

	if err != nil {
		util.LOGGER.Error("POST /api/auth/login: failed to make login request to Directus", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	// Parse response
	var directusResp struct {
		Data LoginResponse `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
		util.LOGGER.Error("POST /api/auth/login: failed to decode response body", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	ctx.JSON(http.StatusOK, directusResp.Data)
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Logout godoc
// @Summary      User logout
// @Description  Invalidate all tokens for logout
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request  body      LogoutRequest  true  "Logout body: refresh token"
// @Success      200  {object}  SuccessMessage  "Successful logout, all tokens is invalidate"
// @Failure      400  {object}  ErrorResponse  "Invalid request body or incorrect credentials"
// @Failure      403  {object}  ErrorResponse  "Account not active, cannot login"
// @Failure      500  {object}  ErrorResponse  "Internal server error"
// @Router       /api/auth/logout [post]
func (server *Server) Logout(ctx *gin.Context) {
	// Get request body
	var req LogoutRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// Make request to Directus
	url := fmt.Sprintf("%s/%s/%s", server.config.DirectusAddr, "auth", "logout")
	_, status, err := util.MakeRequest(
		"POST",
		url,
		map[string]any{"refresh_token": req.RefreshToken},
		server.config.DirectusStaticToken,
	)
	if err != nil {
		util.LOGGER.Error("POST /api/auth/logout: failed to make request to Directus", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, SuccessMessage{"Logout success"})
}

func (server *Server) RefreshToken(ctx *gin.Context) {
	var req LogoutRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	url := fmt.Sprintf("%s/%s/%s", server.config.DirectusAddr, "auth", "refresh")
	resp, status, err := util.MakeRequest(
		"POST",
		url,
		map[string]any{"refresh_token": req.RefreshToken},
		server.config.DirectusStaticToken,
	)
	if err != nil {
		util.LOGGER.Error("POST /api/auth/refresh: failed to make request to Directus", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	var directusResp struct {
		Data LoginResponse `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
		util.LOGGER.Error("POST /api/auth/refresh: failed to decode response", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	ctx.JSON(http.StatusOK, directusResp.Data)
}
