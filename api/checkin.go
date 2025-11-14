package api

import (
	"fmt"
	"net/http"
	"strings"
	"tekticket/db"
	"tekticket/service/worker"
	"tekticket/util"
	"time"

	"github.com/gin-gonic/gin"
)

type CheckinRequest struct {
	StaffEmail    string `json:"staff_email" binding:"required"`
	StaffPassword string `json:"staff_password" binding:"required"`
	CheckinDevice string `json:"checkin_device" binding:"required"`
	Token         string `json:"token" binding:"required"`
}

// Checkin godoc
// @Summary      Check in attendee via QR code
// @Description  Allows event staff to verify a QR token, validate the event schedule,
// @Description  and mark a ticket as checked in. Requires staff credentials and Directus authentication.
// @Tags         Checkin
// @Accept       json
// @Produce      json
// @Param        request body CheckinRequest true "Check-in request payload"
// @Success      200  {object}  SuccessMessage  "Check-in successful"
// @Failure      400  {object}  ErrorResponse   "Invalid request body | Checkin time not started yet | Checkin time has ended | QR not available | Invalid request data"
// @Failure      401  {object}  ErrorResponse   "Incorrect login credentials"
// @Failure      403  {object}  ErrorResponse   "You don't have permission to perform this request"
// @Failure      404  {object}  ErrorResponse   "No item with such ID"
// @Failure      429  {object}  ErrorResponse   "You hit the rate limit"
// @Failure      500  {object}  ErrorResponse   "Internal server or Directus error"
// @Router       /api/checkins [post]
func (server *Server) Checkin(ctx *gin.Context) {
	// Get and parse request
	var req CheckinRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Warn("POST /api/checkins: failed to parse request body", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// First, check if the staff information is valid
	url := fmt.Sprintf("%s/auth/login", server.config.DirectusAddr)
	var loginResp LoginResponse
	body := map[string]any{"email": req.StaffEmail, "password": req.StaffPassword}
	status, err := db.MakeRequest("POST", url, body, server.config.DirectusStaticToken, &loginResp)
	if err != nil {
		util.LOGGER.Error("POST /api/checkins: staff credential checkin failed", "status", status, "error", err)
		server.DirectusError(ctx, err)
		return
	}

	// Get the role from access token, and check if this role is staff
	role, err := util.ExtractRoleFromToken(loginResp.AccessToken, server.config.DirectusAddr, server.config.DirectusStaticToken)
	if err != nil {
		util.LOGGER.Error("POST /api/checkins: failed to get requester role", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	if role = strings.ToLower(strings.TrimSpace(role)); role != "staff" {
		util.LOGGER.Warn("POST /api/checkins: invalid role", "role", role)
		ctx.JSON(http.StatusForbidden, ErrorResponse{"You don't have permission to perform this request"})
		return
	}

	// Get staff ID
	staffID, err := util.ExtractIDFromToken(loginResp.AccessToken)
	if err != nil {
		util.LOGGER.Error("POST /api/checkins: failed to get staff ID from access token", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Veirfy token
	bookingItemID, err := worker.VerifyQRToken(req.Token, server.config.SecretKey)
	if err != nil {
		util.LOGGER.Error("POST /api/checkins: failed to verify check in token", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Get booking data
	fields := []string{
		"id", "status", "event_schedule_id.id", "event_schedule_id.start_checkin_time", "event_schedule_id.end_checkin_time",
	}
	url = fmt.Sprintf("%s/items/booking_items/%s?fields=%s", server.config.DirectusAddr, bookingItemID, strings.Join(fields, ","))
	var bookingItem db.BookingItem
	status, err = db.MakeRequest("GET", url, nil, loginResp.AccessToken, &bookingItem)
	if err != nil {
		util.LOGGER.Error("POST /api/checkins: failed to get booking item", "status", status, "error", err)
		server.DirectusError(ctx, err)
		return
	}

	// Check if this is in the checkin time frame
	now := time.Now()
	if bookingItem.EventSchedule == nil {
		util.LOGGER.Error("POST /api/checkins: event schedule of booking item is nil")
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	if bookingItem.EventSchedule.StartCheckinTime == nil {
		util.LOGGER.Error("POST /api/checkins: start checkin time is nil")
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	if bookingItem.EventSchedule.EndCheckinTime == nil {
		util.LOGGER.Error("POST /api/checkins: end checkin time is nil")
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	startCheckinTime := time.Time(*bookingItem.EventSchedule.StartCheckinTime)
	if now.Before(startCheckinTime) {
		util.LOGGER.Warn(
			"POST /api/checkins: current time is before start checkin time",
			"now", now.String(),
			"start_checkin_time", startCheckinTime.String(),
		)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Checkin time not started yet"})
		return
	}

	endCheckinTime := time.Time(*bookingItem.EventSchedule.EndCheckinTime)
	if now.After(endCheckinTime) {
		util.LOGGER.Warn(
			"POST /api/checkins: now has passed checkin time",
			"now", now.String(),
			"end_checkin_time", endCheckinTime.String(),
		)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Checkin time has ended"})
		return
	}

	// Check if QR status is still available
	if bookingItem.Status != "available" {
		util.LOGGER.Warn("POST /api/checkins: QR status not available", "status", bookingItem.Status)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"QR not available"})
		return
	}

	// Create checkin record in database
	url = fmt.Sprintf("%s/items/checkins", server.config.DirectusAddr)
	body = map[string]any{
		"staff_id":        staffID,
		"booking_item_id": bookingItem.ID,
		"device":          req.CheckinDevice,
	}
	status, err = db.MakeRequest("POST", url, body, loginResp.AccessToken, nil)
	if err != nil {
		util.LOGGER.Error("POST /api/checkins: failed to create checkin record in database", "status", status, "error", err)
		server.DirectusError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, SuccessMessage{"Check in success"})
}
