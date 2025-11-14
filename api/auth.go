package api

import (
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
// @Description  Creates a new user in Directus and triggers a verification email. The email must be unique per role.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body RegisterRequest true "User registration information"
// @Success      200 {object} RegisterResponse "Account created successfully"
// @Failure      400 {object} ErrorResponse "Invalid request body | Invalid role value | Email already registered | Invalid request data"
// @Failure      429 {object} ErrorResponse "Rate limit exceeded"
// @Failure      500 {object} ErrorResponse "Internal server error | Failed to send verification email"
// @Router       /api/auth/register [post]
func (server *Server) Register(ctx *gin.Context) {
	// Get request body and validate
	var req RegisterRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Warn("POST /api/auth/register: failed to bind request body", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// Check roles
	var roles []db.Role
	url := fmt.Sprintf("%s/roles?fields=id,name,description&filter[name][_icontains]=%s", server.config.DirectusAddr, req.Role)
	status, err := db.MakeRequest("GET", url, nil, server.config.DirectusStaticToken, &roles)
	if err != nil {
		util.LOGGER.Error("POST /api/auth/register: failed to get the list of roles for validation", "status", status, "error", err)
		server.DirectusError(ctx, err)
		return
	}

	if len(roles) == 0 {
		util.LOGGER.Warn("POST /api/auth/register: request role invalid, cannot found any role with this name", "role", req.Role)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid role value"})
		return
	}

	// Check if this email has been register. Email must be unique for each role
	url = fmt.Sprintf(
		"%s/users?fields=id&filter[email][_eq]=%s&filter[role][name][_icontains]=%s",
		server.config.DirectusAddr,
		req.Email,
		req.Role,
	)
	var users []db.User
	status, err = db.MakeRequest("GET", url, nil, server.config.DirectusStaticToken, &users)
	if err != nil {
		util.LOGGER.Error(
			"POST /api/auth/register: failed to get the list of users to check if email has been registered",
			"status", status,
			"error", err,
		)
		server.DirectusError(ctx, err)
		return
	}

	if len(users) != 0 {
		util.LOGGER.Warn("POST /api/auth/register: email with this role has already exists", "email", req.Email, "role", req.Role)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Email already registered"})
		return
	}

	// Make request to directus server
	fields := []string{"id", "first_name", "last_name", "email", "role.name", "status"}
	url = fmt.Sprintf("%s/users?fields=%s", server.config.DirectusAddr, strings.Join(fields, ","))
	body := map[string]any{
		"first_name": req.Firstname,
		"last_name":  req.Lastname,
		"email":      req.Email,
		"password":   req.Password,
		"role":       roles[0].ID,
		"status":     "unverified",
	}
	var user db.User
	status, err = db.MakeRequest("POST", url, body, server.config.DirectusStaticToken, &user)
	if err != nil {
		util.LOGGER.Error("POST /api/auth/register: failed to create new user", "status", status, "error", err)
		server.DirectusError(ctx, err)
		return
	}

	// Create background task: send verify email
	err = server.distributor.DistributeTask(ctx, worker.SendVerifyEmail, worker.SendVerifyEmailPayload{
		ID:       user.ID,
		Email:    user.Email,
		Username: fmt.Sprintf("%s %s", user.FirstName, user.LastName),
	}, asynq.Queue(worker.MEDIUM_IMPACT), asynq.MaxRetry(5))

	if err != nil {
		util.LOGGER.Error("POST /api/auth/register: failed to distribute task", "task", worker.SendVerifyEmail, "error", err)
		message := "Create account success, failed to send verify email! Please try using the endpoint: /api/auth/resend-otp"
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{message})
		return
	}

	// return result back to client
	ctx.JSON(http.StatusOK, RegisterResponse{
		ID:        user.ID,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Email:     user.Email,
		Role:      user.Role.Name,
		Status:    user.Status,
	})
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
// @Failure      400  {object}  ErrorResponse   "Invalid OTP code | OTP expired | ID mismatch with OTP"
// @Failure      429  {object}  ErrorResponse   "Rate limit exceeded"
// @Failure      500  {object}  ErrorResponse   "Internal server error"
// @Router       /api/auth/verify/{id} [post]
func (server *Server) VerifyAccount(ctx *gin.Context) {
	// Get user ID and OTP code
	id := ctx.Param("id")
	otp := ctx.Query("otp")

	// OTP validation
	if otp = strings.TrimSpace(otp); len(otp) != 6 {
		util.LOGGER.Warn("POST /api/auth/verify/{id}: invalid otp format", "otp len", len(otp))
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid OTP code"})
		return
	}

	// Get user ID from cache
	idCached, err := server.queries.GetCache(ctx, otp)
	if err != nil && !server.queries.IsCacheMiss(err) {
		// If error, but not a cache miss -> server error
		util.LOGGER.Error("POST /api/auth/verify: failed to get OTP code from cache", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	if idCached == "" {
		// If the returned ID is empty, it would either be an invalid OTP that isn't store in cache, or the OTP expired.
		// But whatever reason, the result is same -> client need to get a new OTP, so we are just telling them that OTP expired
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"OTP expired"})
		return
	}

	// Compare if the ID provided from client, and the cached ID is match
	if idCached != id {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"ID mismatch with OTP"})
		return
	}

	// Update user status to 'active'
	url := fmt.Sprintf("%s/users/%s", server.config.DirectusAddr, id)
	status, err := db.MakeRequest("PATCH", url, map[string]any{"status": "active"}, server.config.DirectusStaticToken, nil)
	if err != nil {
		// Internal operation -> always return 500
		util.LOGGER.Error("POST /api/auth/verify: failed to update account status", "status", status, "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
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
// @Failure      400  {object}  ErrorResponse   "Account status not unverified"
// @Failure      404  {object}  ErrorResponse   "No item with such ID"
// @Failure      429  {object}  ErrorResponse   "Rate limit exceeded"
// @Failure      500  {object}  ErrorResponse   "Internal server error"
// @Router       /api/auth/resend-otp/{id} [post]
func (server *Server) ResendOTP(ctx *gin.Context) {
	// Get ID from path parameter
	id := ctx.Param("id")

	// Check if this user exists
	url := fmt.Sprintf("%s/users/%s?fields=id,email,first_name,last_name,status", server.config.DirectusAddr, id)
	var user db.User
	status, err := db.MakeRequest("GET", url, nil, server.config.DirectusStaticToken, &user)
	if err != nil {
		util.LOGGER.Error(
			"POST /api/auth/resend-otp/{id}: failed to get user information for OTP resend",
			"status", status,
			"id", id,
			"error", err,
		)
		server.DirectusError(ctx, err)
		return
	}

	// Check if account status is unverified
	if user.Status != "unverified" {
		util.LOGGER.Warn("POST /api/auth/resend-otp/{id}: user status not unverified, skip this request", "status", user.Status)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Account status not unverified"})
		return
	}

	// Create background job, send OTP
	err = server.distributor.DistributeTask(ctx, worker.SendVerifyEmail, worker.SendVerifyEmailPayload{
		ID:       user.ID,
		Email:    user.Email,
		Username: fmt.Sprintf("%s %s", user.FirstName, user.LastName),
	}, asynq.Queue(worker.HIGH_IMPACT), asynq.MaxRetry(5))

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
	ID           string `json:"id"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Expires      int    `json:"expires"`
}

// Login godoc
// @Summary      User login
// @Description  Authenticates a user with Directus and returns an access token and refresh token.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body LoginRequest true "User login credentials"
// @Success      200 {object} LoginResponse "Login successful"
// @Failure      400 {object} ErrorResponse "Invalid request body"
// @Failure      401 {object} ErrorResponse "Incorrect login credentials"
// @Failure      429 {object} ErrorResponse "Rate limit exceeded"
// @Failure      500 {object} ErrorResponse "Internal server error"
// @Router       /api/auth/login [post]
func (server *Server) Login(ctx *gin.Context) {
	// Get request body
	var req LoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Warn("POST /api/auth/login: failed to bind request body", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// Call login request to Directus
	url := fmt.Sprintf("%s/auth/login", server.config.DirectusAddr)
	var result LoginResponse
	status, err := db.MakeRequest("POST", url, map[string]any{
		"email":    req.Email,
		"password": req.Password,
	}, server.config.DirectusStaticToken, &result)

	if err != nil {
		util.LOGGER.Error("POST /api/auth/login: failed to make login", "status", status, "error", err)
		server.DirectusError(ctx, err)
		return
	}

	// Get user ID from access token.
	// Note that JWT payload should use base64.RawURLEncoding instead of base64.URLEncoding
	// Even if this failed for some reasons, the consumer (client) can still get the user ID from the JWT access token, so we won't
	// return error here.
	if id, err := util.ExtractIDFromToken(result.AccessToken); err == nil {
		result.ID = id
	} else {
		util.LOGGER.Error("POST /api/auth/login: failed to decode JWT payload", "error", err)
	}

	ctx.JSON(http.StatusOK, result)
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Logout godoc
// @Summary      Logout user
// @Description  Logs out a user by invalidating the provided refresh token in Directus.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body LogoutRequest true "Refresh token for logout"
// @Success      200 {object} SuccessMessage "Logout success"
// @Failure      400 {object} ErrorResponse "Invalid request body"
// @Failure      403 {object} ErrorResponse "Invalid token"
// @Failure      429 {object} ErrorResponse "Rate limit exceeded"
// @Failure      500 {object} ErrorResponse "Internal server error"
// @Router       /api/auth/logout [post]
func (server *Server) Logout(ctx *gin.Context) {
	// Get request body
	var req LogoutRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Warn("POST /api/auth/logout: failed to bind request body", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// Make request to Directus
	url := fmt.Sprintf("%s/auth/logout", server.config.DirectusAddr)
	status, err := db.MakeRequest(
		"POST",
		url,
		map[string]any{"refresh_token": req.RefreshToken},
		server.config.DirectusStaticToken,
		nil,
	)
	if err != nil {
		util.LOGGER.Error("POST /api/auth/logout: failed to logout", "status", status, "error", err)
		server.DirectusError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, SuccessMessage{"Logout success"})
}

// RefreshToken godoc
// @Summary      Refresh access token
// @Description  Refreshes the access token using a valid Directus refresh token.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body LogoutRequest true "Refresh token for refreshing access token"
// @Success      200 {object} LoginResponse "Token refresh success"
// @Failure      400 {object} ErrorResponse "Invalid request body"
// @Failure      403 {object} ErrorResponse "Invalid token"
// @Failure      429 {object} ErrorResponse "Rate limit exceeded"
// @Failure      500 {object} ErrorResponse "Internal server error"
// @Router       /api/auth/refresh [post]
func (server *Server) RefreshToken(ctx *gin.Context) {
	// Get request body
	var req LogoutRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Warn("POST /api/auth/refresh: failed to bind request body", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// Make request to Directus
	url := fmt.Sprintf("%s/auth/refresh", server.config.DirectusAddr)
	var result LoginResponse
	body := map[string]any{"refresh_token": req.RefreshToken}
	status, err := db.MakeRequest("POST", url, body, server.config.DirectusStaticToken, &result)
	if err != nil {
		util.LOGGER.Error("POST /api/auth/refresh: failed to refresh token", "status", status, "error", err)
		server.DirectusError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// SendResetPasswordRequest godoc
// @Summary      Send password reset request
// @Description  Sends a password reset email to the specified email address if the account exists.
// @Description  The email will contain a link or OTP to reset the user's password.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        email  query     string  true  "User email address"
// @Success      200  {object}  SuccessMessage  "Email sent successfully"
// @Failure      400  {object}  ErrorResponse   "No account with this email | Email cannot be empty"
// @Failure      404  {object}  ErrorResponse   "No item with such ID"
// @Failure      429  {object}  ErrorResponse   "You hit the rate limit"
// @Failure      500  {object}  ErrorResponse   "Internal server error"
// @Router       /api/auth/password/request [post]
func (server *Server) SendResetPasswordRequest(ctx *gin.Context) {
	// Get email and role from query parameter and validate
	email := ctx.Query("email")
	role := ctx.Query("role")

	if email = strings.TrimSpace(email); email == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Email cannot be empty"})
		return
	}

	if role = strings.TrimSpace(role); role == "" {
		util.LOGGER.Warn("POST /api/auth/password/request: role not provided, used customer as default")
		role = "customer"
	}

	// Get the user with provided ID
	url := fmt.Sprintf(
		"%s/users?fields=id,email&filter[email][_eq]=%s&filter[role][name][_icontains]=%s",
		server.config.DirectusAddr,
		email,
		role,
	)
	var users []db.User
	status, err := db.MakeRequest("GET", url, nil, server.config.DirectusStaticToken, &users)
	if err != nil {
		util.LOGGER.Error(
			"POST /api/auth/password/request: failed to get list of user with provided email and role",
			"status", status,
			"error", err,
		)
		server.DirectusError(ctx, err)
		return
	}

	if len(users) == 0 {
		util.LOGGER.Warn("POST /api/auth/password/request: no account with this email and role", "email", email, "role", role)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"No acount with this email"})
		return
	}

	// Create background task: send reset password request
	err = server.distributor.DistributeTask(ctx, worker.SendResetPassword, worker.SendResetPasswordPayload{
		ID:    users[0].ID,
		Email: users[0].Email,
	}, asynq.Queue(worker.MEDIUM_IMPACT), asynq.MaxRetry(5))

	if err != nil {
		util.LOGGER.Error("POST /api/auth/password/request: failed to distribute task", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	ctx.JSON(http.StatusOK, SuccessMessage{"Email sent successfully"})
}

type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// ResetPassword godoc
// @Summary      Reset user password
// @Description  Resets the user's password using a valid reset token. The token must be verified before updating the password.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body ResetPasswordRequest true "Token and new password"
// @Success      200 {object} SuccessMessage "Password change successfully"
// @Failure      400 {object} ErrorResponse "Invalid request body | Invalid request data"
// @Failure      429 {object} ErrorResponse "Rate limit exceeded"
// @Failure      500 {object} ErrorResponse "Internal server error"
// @Router       /api/auth/password/reset [post]
func (server *Server) ResetPassword(ctx *gin.Context) {
	// Get the payload
	var req ResetPasswordRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Warn("POST /api/auth/password/reset: failed to bind request body", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Verify token
	payload, err := worker.VerifyResetPasswordToken(req.Token, server.config.SecretKey)
	if err != nil {
		util.LOGGER.Error("POST /api/auth/password/reset: failed to verify token", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Update password
	url := fmt.Sprintf("%s/users/%s", server.config.DirectusAddr, payload[0])
	status, err := db.MakeRequest("PATCH", url, map[string]any{"password": req.NewPassword}, server.config.DirectusStaticToken, nil)
	if err != nil {
		util.LOGGER.Error("POST /api/auth/password/reset: failed to reset password", "status", status, "error", err)
		server.DirectusError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, SuccessMessage{"Password change successfully"})
}
