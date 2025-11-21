package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"tekticket/db"
	"tekticket/util"

	"github.com/gin-gonic/gin"
)

// ListNotification godoc
// @Summary      List all notifications
// @Description  Returns a list of notifications
// @Tags         Notification
// @Accept       json
// @Produce      json
// @Success      200  {array}   db.NotificationRecipent         "List of events retrieved successfully"
// @Failure      401  {object}  ErrorResponse     				"Unauthorized access | Token expired"
// @Failure      403  {object}  ErrorResponse     				"Invalid token"
// @Failure      429  {object}  ErrorResponse     				"You hit the rate limit"
// @Failure      500  {object}  ErrorResponse     				"Internal server error"
// @Security     BearerAuth
// @Router       /api/notifications/me [get]
func (server *Server) ListNotification(ctx *gin.Context) {
	// Get token
	token := server.GetToken(ctx)

	// Get user ID from access token
	userID, err := util.ExtractIDFromToken(token)
	if err != nil {
		util.LOGGER.Error("GET /api/notifications/me: failed to extract user ID from access token", "error", err)
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Invalid access token"})
		return
	}

	// Build the URL
	queryParams := &url.Values{}
	fields := []string{"id", "date_created", "notification_id.id", "notification_id.message"}
	queryParams.Add("fields", strings.Join(fields, ","))
	queryParams.Add("filter[receiver_id][_eq]", userID)

	// Pagination
	limit := 50
	if val, err := strconv.Atoi(ctx.Query("limit")); err == nil && val > 0 {
		limit = val
	}
	queryParams.Add("limit", strconv.Itoa(limit))

	offset := 0
	if val, err := strconv.Atoi(ctx.Query("offset")); err == nil && val >= 0 {
		offset = val
	}
	queryParams.Add("offset", strconv.Itoa(offset))

	// Sort
	sort := ctx.Query("sort")
	if sort == "" {
		sort = "-date_created" // Default: newest first
	}
	queryParams.Add("sort", sort)

	url := fmt.Sprintf("%s/items/notification_recipents?%s", server.config.DirectusAddr, queryParams.Encode())

	// Make request
	var notifications []db.NotificationRecipent
	status, err := db.MakeRequest("GET", url, nil, token, &notifications)
	if err != nil {
		util.LOGGER.Error("GET /api/notifications/me: failed to get notifications", "status", status, "error", err)
		server.DirectusError(ctx, err)
		return
	}

	// Execute templete into a message

	ctx.JSON(http.StatusOK, notifications)
}
