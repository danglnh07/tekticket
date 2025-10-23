package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"tekticket/util"

	"github.com/gin-gonic/gin"
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
// @Success      200  {object}  ProfileResponse  "user profile"
// @Failure      400  {object}  ErrorResponse  "Invalid request body or incorrect credentials"
// @Failure      403  {object}  ErrorResponse  "Account not active, cannot login"
// @Failure      500  {object}  ErrorResponse  "Internal server error"
// @Security     BearerAuth
// @Router       /api/profile [get]
func (server *Server) GetProfile(ctx *gin.Context) {
	// Get user
	url := fmt.Sprintf("%s/%s/%s", server.config.DirectusAddr, "users", "me")
	resp, status, err := util.MakeRequest("GET", url, nil, server.GetToken(ctx))
	if err != nil {
		util.LOGGER.Error("GET /api/profile: failed to make request to Directus", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	// Parse request
	var directusResp struct {
		Data ProfileResponse `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
		util.LOGGER.Error("GET /api/profile: failed to parse response", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	var profile = ProfileResponse{
		ID:        directusResp.Data.ID,
		Firstname: directusResp.Data.Firstname,
		Lastname:  directusResp.Data.Lastname,
		Email:     directusResp.Data.Email,
		Location:  directusResp.Data.Location,
		Avatar:    directusResp.Data.Avatar,
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
// @Description  The avatar is expected to be a base64-encoded image, which will be uploaded to cloud storage and replaced with its secure URL.
// @Tags         Profile
// @Accept       json
// @Produce      json
// @Param        request  body      UpdateProfileRequest  true  "Profile update request body"
// @Success      200  {object}  ProfileResponse  "Profile updated successfully"
// @Failure      400  {object}  ErrorResponse   "Invalid request body"
// @Failure      500  {object}  ErrorResponse   "Internal server error"
// @Security     BearerAuth
// @Router       /api/profile [put]
func (server *Server) UpdateProfile(ctx *gin.Context) {
	// Get request body
	var req UpdateProfileRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
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
		avatarID, err := server.UploadImage(ctx, req.Avatar)
		if err != nil {
			return
		}

		data["avatar"] = avatarID
	}

	// Make request to Directus API
	url := fmt.Sprintf("%s/%s/%s", server.config.DirectusAddr, "users", "me")
	resp, status, err := util.MakeRequest("PATCH", url, data, server.GetToken(ctx))
	if err != nil {
		util.LOGGER.Error("PUT /api/profile: failed to make request into Directus", "error", err)
		ctx.JSON(status, ErrorResponse{resp.Status})
		return
	}

	// Parse response return
	var directusResp struct {
		Data ProfileResponse `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
		util.LOGGER.Error("PUT /api/profile: failed to parse response body", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal servere error"})
		return
	}

	ctx.JSON(http.StatusOK, directusResp.Data)
}
