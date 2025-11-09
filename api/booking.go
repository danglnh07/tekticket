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

// ListBookingHistory godoc
// @Summary      Get user's booking history
// @Description  Retrieves the list of completed bookings for the authenticated user, including event and category details.
// @Tags        Bookings
// @Accept       json
// @Produce      json
// @Param        limit          query     int     false  "Maximum number of records to return (default: 50)"
// @Param        offset         query     int     false  "Number of records to skip before returning results (default: 0)"
// @Param        sort           query     string  false  "Sort order, e.g. -date_created (default)"
// @Success      200  {array}   []db.Booking    "List of completed bookings retrieved successfully"
// @Failure      400  {object}  ErrorResponse     "Invalid token or parameters"
// @Failure      401  {object}  ErrorResponse     "Unauthorized access"
// @Failure      500  {object}  ErrorResponse     "Internal server error or failed to communicate with Directus"
// @Security BearerAuth
// @Router       /api/bookings [get]
func (server *Server) ListBookingHistory(ctx *gin.Context) {
	// Get access token
	token := server.GetToken(ctx)
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Unauthorized access"})
		return
	}

	// Get user ID from access token
	id, err := util.ExtractIDFromToken(token)
	if err != nil {
		util.LOGGER.Error("GET /api/profile/bookings: failed to extract user ID from access token", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid token"})
		return
	}

	// Build the URL query
	queryParams := url.Values{}
	fields := []string{
		"id",
		"event_id.id", "event_id.name", "event_id.address", "event_id.city", "event_id.country", "event_id.preview_image",
		"event_id.event_schedules.id", "event_id.event_schedules.start_time", "event_id.event_schedules.end_time",
		"event_id.event_schedules.start_checkin_time", "event_id.event_schedules.end_checkin_time",
		"event_id.category_id.id", "event_id.category_id.name", "event_id.category_id.description",
	}
	queryParams.Add("fields", strings.Join(fields, ","))
	queryParams.Add("filter[customer_id][_eq]", id)
	queryParams.Add("filter[status][_icontains]", "complete")

	// Pagination
	limit := 50
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil && val > 0 {
			limit = val
		}
	}
	queryParams.Add("limit", strconv.Itoa(limit))

	offset := 0
	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		if val, err := strconv.Atoi(offsetStr); err == nil && val >= 0 {
			offset = val
		}
	}
	queryParams.Add("offset", strconv.Itoa(offset))

	// Sort
	sort := ctx.Query("sort")
	if sort == "" {
		sort = "-date_created" // Default: newest first
	}
	queryParams.Add("sort", sort)

	// Build the URL
	directusURL := fmt.Sprintf("%s/items/bookings?%s", server.config.DirectusAddr, queryParams.Encode())

	// Make request to Directus
	var results []db.Booking
	status, err := db.MakeRequest("GET", directusURL, nil, token, &results)
	if err != nil {
		util.LOGGER.Error("GET /api/bookings/:id: failed to get booking history from Directus", "error", err)
		ctx.JSON(status, ErrorResponse{Message: err.Error()})
		return
	}

	// Remap preview_image of event
	for _, result := range results {
		if result.Event.PreviewImage != "" {
			result.Event.PreviewImage = util.CreateImageLink(server.config.ServerDomain, result.Event.PreviewImage)
		}
	}

	ctx.JSON(http.StatusOK, results)
}

// GetBooking godoc
// @Summary      Get booking detail
// @Description  Retrieves the detailed information of a specific booking, including event details, schedules, and ticket data.
// @Tags         Bookings
// @Accept       json
// @Produce      json
// @Param        id             path      string  true   "Booking ID"
// @Success      200  {object}  db.Booking     "Booking detail retrieved successfully"
// @Failure      400  {object}  ErrorResponse     "Invalid request parameters"
// @Failure      401  {object}  ErrorResponse     "Unauthorized access"
// @Failure      404  {object}  ErrorResponse     "Booking not found"
// @Failure      500  {object}  ErrorResponse     "Internal server error or failed to communicate with Directus"
// @Security BearerAuth
// @Router       /api/bookings/{id} [get]
func (server *Server) GetBooking(ctx *gin.Context) {
	// Get access token
	token := server.GetToken(ctx)
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Unauthorized access"})
		return
	}

	// Get booking ID from path paramter
	id := ctx.Param("id")

	// Build the query parameter
	queryParams := url.Values{}
	fields := []string{
		"id", "status", "date_created",
		"event_id.id", "event_id.name", "event_id.address", "event_id.city", "event_id.country", "event_id.preview_image",
		"event_id.event_schedules.id", "event_id.event_schedules.start_time", "event_id.event_schedules.end_time",
		"event_id.event_schedules.start_checkin_time", "event_id.event_schedules.end_checkin_time",
		"event_id.category_id.id", "event_id.category_id.name", "event_id.category_id.description",
		"booking_items.id", "booking_items.price", "booking_items.qr",
		"booking_items.seat_id.id", "booking_items.seat_id.seat_number",
	}
	queryParams.Add("fields", strings.Join(fields, ","))

	// Make request to Directus
	url := fmt.Sprintf("%s/items/bookings/%s?%s", server.config.DirectusAddr, id, queryParams.Encode())
	var result db.Booking
	status, err := db.MakeRequest("GET", url, nil, token, &result)
	if err != nil {
		util.LOGGER.Error("GET /api/bookings/:id: failed to get booking detail from Directus", "error", err, "id", id)
		ctx.JSON(status, ErrorResponse{Message: err.Error()})
		return
	}

	// Remap image for event and tickets' QRs
	if result.Event.PreviewImage != "" {
		result.Event.PreviewImage = util.CreateImageLink(server.config.ServerDomain, result.Event.PreviewImage)
	}

	for i, ticket := range result.BookingItems {
		if ticket.QR != "" {
			result.BookingItems[i].QR = util.CreateImageLink(server.config.ServerDomain, ticket.QR)

		}
	}

	ctx.JSON(http.StatusOK, result)
}

type BookingItemCreate struct {
	TicketID        string `json:"ticket_id" binding:"required"`
	EventScheduleID string `json:"event_schedule_id" binding:"required"`
	SeatID          string `json:"seat_id" binding:"required"`
}

type CreateBookingRequest struct {
	EventID string              `json:"event_id" binding:"required"`
	Items   []BookingItemCreate `json:"items" binding:"required,min=1,dive"`
}

type CreateBookingResponse struct {
	ID             string           `json:"id"`
	Status         string           `json:"status"`
	Event          db.Event         `json:"event"`
	Customer       db.User          `json:"customer"`
	Tickets        []db.BookingItem `json:"tickets"`
	TotalPricePaid int              `json:"total_price_paid"`
	FeeCharged     int              `json:"fee_charged"`
}

// CreateBooking godoc
// @Summary      Create a new booking
// @Description  Creates a new booking for an event, including its associated ticket and seat items.
// @Tags         Bookings
// @Accept       json
// @Produce      json
// @Param        request        body    CreateBookingRequest   true   "Booking creation payload"
// @Success      200  {object}  CreateBookingResponse                  "Booking created successfully"
// @Failure      400  {object}  ErrorResponse                   "Invalid request body"
// @Failure      401  {object}  ErrorResponse                   "Unauthorized access"
// @Failure      500  {object}  ErrorResponse                   "Internal server error or failed to communicate with Directus"
// @Security BearerAuth
// @Router       /api/bookings [post]
func (server *Server) CreateBooking(ctx *gin.Context) {
	// Get access token and extract user
	token := server.GetToken(ctx)
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{Message: "Unauthorized access"})
		return
	}

	// Extract user ID from token
	userID, err := util.ExtractIDFromToken(token)
	if err != nil {
		util.LOGGER.Error("POST /api/bookings: failed to get userID from access token", "error", err)
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Invalid token"})
		return
	}

	// Parse request
	var req CreateBookingRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// Create booking with all items
	payload := map[string]any{
		"customer_id": userID,
		"event_id":    req.EventID,
		"status":      "pending",
	}
	items := make([]map[string]any, 0)
	for _, item := range req.Items {
		items = append(items, map[string]any{
			"ticket_id":         item.TicketID,
			"seat_id":           item.SeatID,
			"event_schedule_id": item.EventScheduleID,
		})
	}
	payload["booking_items"] = items
	fields := []string{
		"id", "date_created", "status",
		"customer_id.id", "customer_id.first_name", "customer_id.last_name", "customer_id.email",
		"event_id.id", "event_id.name", "event_id.address", "event_id.city", "event_id.country", "event_id.preview_image",
		"event_id.event_schedules.id", "event_id.event_schedules.start_time", "event_id.event_schedules.end_time",
		"event_id.event_schedules.start_checkin_time", "event_id.event_schedules.end_checkin_time",
		"event_id.category_id.id", "event_id.category_id.name", "event_id.category_id.description",
		"booking_items.id", "booking_items.price",
		"booking_items.seat_id.id", "booking_items.seat_id.seat_number",
	}
	url := fmt.Sprintf("%s/items/bookings?fields=%s", server.config.DirectusAddr, strings.Join(fields, ","))
	var result db.Booking
	statusCode, err := db.MakeRequest("POST", url, payload, token, &result)
	if err != nil {
		util.LOGGER.Error("POST /api/bookings: failed to create booking", "error", err)
		ctx.JSON(statusCode, ErrorResponse{err.Error()})
		return
	}

	// Remap event's preview image
	if result.Event.PreviewImage != "" {
		result.Event.PreviewImage = util.CreateImageLink(server.config.ServerDomain, result.Event.PreviewImage)
	}

	booking := CreateBookingResponse{
		ID:       result.ID,
		Status:   result.Status,
		Event:    *result.Event,
		Customer: *result.Customer,
		Tickets:  result.BookingItems,
	}

	// Calculate total price paid: sum of all booking_item.price
	for _, item := range result.BookingItems {
		booking.TotalPricePaid += item.Price
	}
	util.LOGGER.Info("POST /api/payments", "id", booking.ID, "before charged", booking.TotalPricePaid)

	booking.FeeCharged = int(float64(server.config.PaymentFeePercent) * float64(booking.TotalPricePaid) / 100)
	booking.TotalPricePaid += booking.FeeCharged
	util.LOGGER.Info("POST /api/payments", "id", booking.ID, "after charged", booking.TotalPricePaid)

	ctx.JSON(http.StatusOK, booking)
}
