package api

import (
	"errors"
	"net/http"
	"tekticket/db"
	"tekticket/service/security"
	"tekticket/service/worker"
	"tekticket/util"

	"github.com/gin-gonic/gin"
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
	Message string `json:"message"`
}

// Register godoc
// @Summary      Register a new user account
// @Description  Creates a new user account with the provided username, email, phone, password, and role.
// @Description  Sends a verification email to activate the account.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request  body      RegisterRequest  true  "Registration request body"
// @Success      201  {object}  RegisterResponse  "Account created successfully, please activate your account through the email we have sent"
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
	account.Username = req.Username
	account.Email = req.Email
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
	}, asynq.MaxRetry(1))
	if err != nil {
		util.LOGGER.Error("POST /api/auth/register: failed to distribute send verification mail task", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Account created successfully, but failed to send verification email"})
		return
	}

	// Return result back to client
	ctx.JSON(http.StatusCreated, RegisterResponse{
		"Account created successfully, please activate your account through the email we have sent",
	})
}
