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
	// Get the JWT token
	token := strings.TrimPrefix(ctx.Request.Header.Get("Authorization"), "Bearer ")

	// Get user
	url := fmt.Sprintf("%s/%s/%s", server.config.DirectusAddr, "users", "me")
	resp, err := util.MakeRequest("GET", url, nil, token)
	if err != nil {
		util.LOGGER.Error("GET /api/profile: failed to make request to Directus", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
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
		Avatar:    directusResp.Data.Avatar,
	}

	ctx.JSON(http.StatusOK, profile)
}
