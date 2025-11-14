package api

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"tekticket/db"
	"tekticket/util"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ProfileResponse struct {
	ID        string `json:"id"`
	Firstname string `json:"first_name"`
	Lastname  string `json:"last_name"`
	Email     string `json:"email"`
	Location  string `json:"location"`
	Avatar    string `json:"avatar"`
}

// Logout godoc
// @Summary      User profile
// @Description  Get user profile
// @Tags         Profile
// @Accept       json
// @Produce      json
// @Success      200  {object}  ProfileResponse      "User profile"
// @Failure      401  {object}  ErrorResponse        "Token expired"
// @Failure      403  {object}  ErrorResponse        "Invalid token"
// @Failure      429  {object}  ErrorResponse        "You hit the rate limit"
// @Failure      500  {object}  ErrorResponse        "Internal server error"
// @Security     BearerAuth
// @Router       /api/profile [get]
func (server *Server) GetProfile(ctx *gin.Context) {
	// Get user
	url := fmt.Sprintf("%s/%s/%s?fields=id,first_name,last_name,email,location,avatar", server.config.DirectusAddr, "users", "me")
	var profile ProfileResponse
	status, err := db.MakeRequest("GET", url, nil, server.GetToken(ctx), &profile)
	if err != nil {
		util.LOGGER.Error("GET /api/profile: failed to get user profile", "status", status, "error", err)
		server.DirectusError(ctx, err)
		return
	}

	// Remap avatar into an usable link
	if profile.Avatar != "" {
		profile.Avatar = util.CreateImageLink(server.config.ServerDomain, profile.Avatar)
	}

	ctx.JSON(http.StatusOK, profile)
}

type UpdateProfileRequest struct {
	Firstname string `json:"first_name"`
	Lastname  string `json:"last_name"`
	Password  string `json:"password"`
	Location  string `json:"location"`
	Avatar    string `json:"avatar"`
}

// UpdateProfile godoc
// @Summary      Update user profile
// @Description  Updates the current user's profile information including first name, last name, password, and avatar.
// @Description  The avatar is expected to be a base64-encoded image
// @Tags         Profile
// @Accept       json
// @Produce      json
// @Param        request  body  UpdateProfileRequest true  "Profile update request body"
// @Success      200  {object}  ProfileResponse            "Profile updated successfully"
// @Failure      400  {object}  ErrorResponse              "Invalid request body"
// @Failure      401  {object}  ErrorResponse              "Token expired"
// @Failure      403  {object}  ErrorResponse              "Invalid token"
// @Failure      429  {object}  ErrorResponse              "You hit the rate limit"
// @Failure      500  {object}  ErrorResponse              "Internal server error"
// @Security     BearerAuth
// @Router       /api/profile [put]
func (server *Server) UpdateProfile(ctx *gin.Context) {
	// Get request body
	var req UpdateProfileRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Warn("PUT /api/profile: failed to bind request body", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// Check for each field in he request body and construct the payload map
	data := make(map[string]any, 0)

	if req.Firstname = strings.TrimSpace(req.Firstname); req.Firstname != "" {
		data["first_name"] = req.Firstname
	}

	if req.Lastname = strings.TrimSpace(req.Lastname); req.Lastname != "" {
		data["last_name"] = req.Lastname
	}

	if req.Location = strings.TrimSpace(req.Location); req.Location != "" {
		data["location"] = req.Location
	}

	if req.Password = strings.TrimSpace(req.Password); req.Password != "" {
		data["password"] = req.Password
	}

	if req.Avatar = strings.TrimSpace(req.Avatar); req.Avatar != "" {
		// Read the base64 image into a slice of byte
		image, err := base64.StdEncoding.DecodeString(req.Avatar)
		if err != nil {
			util.LOGGER.Warn("PUT /api/profile: failed to decode avatar", "error", err)
			ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid base64 image value for avatar"})
			return
		}

		avatarID, status, err := server.uploadService.Upload(uuid.New().String(), image) // The image doesn't really matter here
		if err != nil {
			util.LOGGER.Error("PUT /api/profile: failed to upload new avatar image", "status", status, "error", err)
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"failed to handle avatar image"})
			return
		}

		data["avatar"] = avatarID
	}

	// Make request to Directus API
	url := fmt.Sprintf("%s/%s/%s", server.config.DirectusAddr, "users", "me")
	var profile ProfileResponse
	status, err := db.MakeRequest("PATCH", url, data, server.GetToken(ctx), &profile)
	if err != nil {
		util.LOGGER.Error("PUT /api/profile: failed to update user profile", "status", status, "error", err)
		server.DirectusError(ctx, err)
		return
	}

	// Remap image link
	if profile.Avatar != "" {
		profile.Avatar = util.CreateImageLink(server.config.ServerDomain, profile.Avatar)
	}

	ctx.JSON(http.StatusOK, profile)
}
