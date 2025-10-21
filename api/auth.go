package api

import (
	"errors"
	"math"
	"net/http"
	"sort"
	"strings"
	"tekticket/db"
	"tekticket/service/security"
	"tekticket/service/worker"
	"tekticket/util"

	"github.com/gin-gonic/gin"
	// "github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Phone    string `json:"phone" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role" binding:"required"`
}

type RegisterResponse struct {
	ID string `json:"id"`
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
		util.LOGGER.Warn("POST /api/auth/register: failed to parse request body", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// Check if role is valid
	role := db.Role(req.Role)
	if role != db.Admin && role != db.Customer && role != db.Organiser && role != db.Staff {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid value for role!"})
		return
	}

	// Check if this username, email and phone exists
	var account db.User
	result := server.queries.DB.
		Where("username = ? OR email = ? OR phone = ?", req.Username, req.Email, req.Phone).
		First(&account)
	if result.Error == nil {
		// If username, email or phone exists
		resp := ErrorResponse{}
		if req.Email == account.Email {
			resp.Message = "This email has been registered. Please login instead"
		} else if req.Username == account.Username {
			resp.Message = "This username has been taken"
		} else if req.Phone == account.Phone {
			resp.Message = "This phone has been registered."
		}

		ctx.JSON(http.StatusBadRequest, resp)
		return
	} else if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		util.LOGGER.Error("POST /api/auth/register: failed to get user data", "error", result.Error)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Hash password
	hashed, err := security.BcryptHash(req.Password)
	if err != nil {
		util.LOGGER.Error("POST /api/auth/register: failed to user password", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Insert into database
	account.Model = db.NewModel()
	account.Username = req.Username
	account.Email = req.Email
	account.Phone = req.Phone
	account.Password = hashed
	account.Role = role
	account.Status = db.Inactive
	result = server.queries.DB.Create(&account)
	if result.Error != nil {
		util.LOGGER.Error("POST /api/auth/register: failed to create new account", "error", result.Error)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Create background task: send verification email to client's email
	err = server.distributor.DistributeTask(ctx, worker.SendVerifyEmail, worker.SendVerifyEmailPayload{
		ID:       account.ID,
		Email:    account.Email,
		Username: account.Username,
	}, asynq.MaxRetry(5))
	if err != nil {
		util.LOGGER.Error("POST /api/auth/register: failed to distribute send verification mail task", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Account created successfully, but failed to send verification email"})
		return
	}

	// Return result back to client
	ctx.JSON(http.StatusCreated, RegisterResponse{ID: account.ID.String()})
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

	// Update account status
	result := server.queries.DB.Table("users").Where("id = ?", id).Update("status", db.Active)
	if result.Error != nil {
		// If ID not match any record
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			ctx.JSON(http.StatusBadRequest, ErrorResponse{"Account with this ID not exists in database"})
			return
		}

		// Other database error
		util.LOGGER.Error("GET /api/auth/verify: failed to update account status", "error", result.Error)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Return result to client
	ctx.JSON(http.StatusOK, SuccessMessage{"Verify account successfully, please login"})
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
func (server *Server) ResendOTP(ctx *gin.Context) {
	// Get ID from path parameter
	id := ctx.Param("id")

	// Check if ID is valid and account status is inactive
	var user db.User
	result := server.queries.DB.Where("id = ?", id).First(&user)
	if result.Error != nil {
		// If ID not match any record
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			ctx.JSON(http.StatusBadRequest, ErrorResponse{"No account with this ID"})
			return
		}

		// Other database error
		util.LOGGER.Error("POST /api/auth/resend-otp: failed to get user data", "error", result.Error)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	if user.Status != db.Inactive {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"account status is not 'inactive', cannot proceed with request"})
		return
	}

	// Create background job, send OTP
	err := server.distributor.DistributeTask(ctx, worker.SendVerifyEmail, worker.SendVerifyEmailPayload{
		ID:       user.ID,
		Email:    user.Email,
		Username: user.Username,
	}, asynq.MaxRetry(5))

	if err != nil {
		util.LOGGER.Error("POST /api/auth/resend-otp: failed to distribute task", "task", worker.SendVerifyEmail, "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	Role         string `json:"role"`
	TotalPoints  uint   `json:"total_points"`
	Membership   string `json:"membership"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
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
	// Get and validate request body
	var req LoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Warn("POST /api/auth/login: failed to parse request body", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// Get user by username
	var user db.User
	result := server.queries.DB.Where("username = ?", req.Username).First(&user)
	if result.Error != nil {
		// If username not match
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			ctx.JSON(http.StatusBadRequest, ErrorResponse{"Incorrect login credential"})
			return
		}

		// Other database error
		util.LOGGER.Error("POST /api/auth/login: failed to get user by username", "error", result.Error)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Check if user status is active
	if user.Status != db.Active {
		ctx.JSON(http.StatusForbidden, ErrorResponse{"Account not active, cannot login"})
		return
	}

	// Check password
	if !security.BcryptCompare(user.Password, req.Password) {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Incorrect login credential"})
		return
	}

	var resp = LoginResponse{
		ID:       user.ID.String(),
		Username: user.Username,
		Email:    user.Email,
		Phone:    user.Phone,
		Role:     string(user.Role),
	}

	// Find the current user point (which is the latest point of the logs) and their corresponding rank
	// Only apply to customer
	if user.Role == db.Customer {
		// Get latest log in database -> current user's point
		var latestLog db.UserMembershipLog
		result = server.queries.DB.
			Where("customer_id = ?", user.ID).
			Order("date_created desc").Limit(1).First(&latestLog)

		if result.Error != nil {
			util.LOGGER.Error("POST /api/auth/login: failed to fetch latest log record", "error", result.Error)
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
			return
		}
		resp.TotalPoints = latestLog.ResultingPoints

		// Get membership
		var memberships []db.Membership
		result = server.queries.DB.Find(&memberships)
		if result.Error != nil {
			util.LOGGER.Error("POST /api/auth/login: failed to get memberships", "error", result.Error)
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
			return
		}

		sort.Slice(memberships, func(i, j int) bool {
			return memberships[i].BasePoint < memberships[j].BasePoint
		})

		var membership string
		for i := range len(memberships) {
			/*
			 * Algorithm explains:
			 * Because base point is defined as uint (and should always be), then the range should be [0, ?], where ? is the highest
			 * rank in database. So we defined into ranges, and check if user point in [lower, higher), then user membership is
			 * memberships[lower]. In the edge case where the current user is at higest point, we set higher = inf
			 */
			lower := memberships[i].BasePoint
			var higher uint

			if i == len(memberships)-1 {
				higher = math.MaxInt
			} else {
				higher = memberships[i+1].BasePoint
			}

			if lower <= resp.TotalPoints && resp.TotalPoints < higher {
				membership = memberships[i].Tier
			}
		}
		resp.Membership = membership
	}

	// Generate tokens
	accessToken, err := server.jwtService.CreateToken(user.ID, user.Role, security.AccessToken, user.TokenVersion)
	if err != nil {
		util.LOGGER.Error("POST /api/auth/login: failed to create access token", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	refreshToken, err := server.jwtService.CreateToken(user.ID, user.Role, security.RefreshToken, user.TokenVersion)
	if err != nil {
		util.LOGGER.Error("POST /api/auth/login: failed to create refresh token")
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Return result back to client
	resp.AccessToken = accessToken
	resp.RefreshToken = refreshToken
	ctx.JSON(http.StatusOK, resp)
}
