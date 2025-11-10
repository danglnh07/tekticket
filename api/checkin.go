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
// @Failure      400  {object}  ErrorResponse   "Invalid QR code or not within check-in timeframe"
// @Failure      401  {object}  ErrorResponse   "Unauthorized or invalid staff role"
// @Failure      404  {object}  ErrorResponse   "Booking item not found"
// @Failure      500  {object}  ErrorResponse   "Internal server or Directus error"
// @Router       /api/checkins [post]
func (server *Server) Checkin(ctx *gin.Context) {
	// Get and parse request
	var req CheckinRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Error("POST /api/checkins: failed to parse request body", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// First, check if the staff information is valid
	url := fmt.Sprintf("%s/auth/login", server.config.DirectusAddr)
	var loginResp LoginResponse
	status, err := db.MakeRequest(
		"POST",
		url,
		map[string]any{
			"email":    req.StaffEmail,
			"password": req.StaffPassword,
		},
		server.config.DirectusStaticToken,
		&loginResp,
	)

	if err != nil {
		util.LOGGER.Error("POST /api/checkins: failed to check staff credential", "status", status, "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	// Get the role from access token, and check if this role is staff
	role, err := util.ExtractRoleFromToken(loginResp.AccessToken, server.config.DirectusAddr, server.config.DirectusStaticToken)
	if err != nil {
		util.LOGGER.Error("POST /api/checkins: failed to get requester role", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Get staff ID
	staffID, err := util.ExtractIDFromToken(loginResp.AccessToken)
	if err != nil {
		util.LOGGER.Error("POST /api/checkins: failed to get staff ID from access token", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	if role = strings.ToLower(strings.TrimSpace(role)); role != "staff" {
		util.LOGGER.Warn("POST /api/checkins: invalid role", "role", role)
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"You don't have permission to perform this request"})
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
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	if status == http.StatusNotFound {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid token, booking item not found"})
		return
	}

	// Check if this is in the checkin time frame
	now := time.Now()
	if bookingItem.EventSchedule != nil &&
		bookingItem.EventSchedule.StartCheckinTime != nil &&
		bookingItem.EventSchedule.EndCheckinTime != nil &&
		now.After(time.Time(*bookingItem.EventSchedule.StartCheckinTime)) &&
		now.Before(time.Time(*bookingItem.EventSchedule.EndCheckinTime)) {

		util.LOGGER.Warn(
			"POST /api/checkins: not checkin time",
			"now", now.String(),
			"start_checkin_time", time.Time(*bookingItem.EventSchedule.StartCheckinTime).String(),
			"end_checkin_time", time.Time(*bookingItem.EventSchedule.EndCheckinTime).String(),
		)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Not checkin time"})
		return
	}

	// Check if QR status is still available
	if bookingItem.Status != "available" {
		util.LOGGER.Warn("POST /api/checkins: QR status not available", "status", bookingItem.Status)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid QR, reject checkin"})
		return
	}

	// Create checkin record in database
	url = fmt.Sprintf("%s/items/checkins", server.config.DirectusAddr)
	body := map[string]any{
		"staff_id":        staffID,
		"booking_item_id": bookingItem.ID,
		"device":          req.CheckinDevice,
	}
	status, err = db.MakeRequest("POST", url, body, loginResp.AccessToken, nil)
	if err != nil {
		util.LOGGER.Error("POST /api/checkins: failed to create checkin record in database", "status", status, "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	ctx.JSON(http.StatusOK, SuccessMessage{"Check in success"})
}
