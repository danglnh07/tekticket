package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"tekticket/util"

	"github.com/gin-gonic/gin"
)

type BookingEventInfo struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Address      string          `json:"address"`
	City         string          `json:"city"`
	Country      string          `json:"country"`
	PreviewImage string          `json:"preview_image"`
	Category     Category        `json:"category"`
	Schedules    []EventSchedule `json:"event_schedules"`
}

type BookingHistory struct {
	ID        string           `json:"id"`
	EventInfo BookingEventInfo `json:"event"`
}

type directusBookingEventInfo struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Address      string          `json:"address"`
	City         string          `json:"city"`
	Country      string          `json:"country"`
	PreviewImage string          `json:"preview_image"`
	Category     Category        `json:"category_id"`
	Schedules    []EventSchedule `json:"event_schedules"`
}

type directusBookingHistory struct {
	ID        string                   `json:"id"`
	EventInfo directusBookingEventInfo `json:"event_id"`
}

// ListBookingHistory godoc
// @Summary      Get user's booking history
// @Description  Retrieves the list of completed bookings for the authenticated user, including event and category details.
// @Tags        Bookings
// @Accept       json
// @Produce      json
// @Param        limit          query     int     false  "Maximum number of records to return (default: 50)"
// @Param        offset         query     int     false  "Number of records to skip before returning results (default: 0)"
// @Param        sort           query     string  false  "Sort order, e.g. -date_created (default)"
// @Success      200  {array}   BookingHistory    "List of completed bookings retrieved successfully"
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
	var directusResult []directusBookingHistory
	statusCode, err := util.MakeRequest("GET", directusURL, nil, token, &directusResult)
	if err != nil {
		util.LOGGER.Error("GET /api/bookings/:id: failed to get booking history from Directus", "error", err)
		ctx.JSON(statusCode, ErrorResponse{Message: err.Error()})
		return
	}

	// Build result
	history := make([]BookingHistory, len(directusResult))
	for i, result := range directusResult {
		history[i] = BookingHistory{
			ID: result.ID,
			EventInfo: BookingEventInfo{
				ID:           result.EventInfo.ID,
				Name:         result.EventInfo.Name,
				Address:      result.EventInfo.Address,
				City:         result.EventInfo.City,
				Country:      result.EventInfo.Country,
				PreviewImage: util.CreateImageLink(result.EventInfo.PreviewImage),
				Category:     result.EventInfo.Category,
				Schedules:    result.EventInfo.Schedules,
			},
		}
	}

	ctx.JSON(http.StatusOK, history)
}

type BookingTicket struct {
	ID    string `json:"id"`
	Price int    `json:"price"`
	QR    string `json:"qr"`
	Seat  Seat   `json:"seat"`
}

type BookingDetail struct {
	ID          string           `json:"id"`
	BookingDate string           `json:"booking_date"`
	EventInfo   BookingEventInfo `json:"event"`
	Tickets     []BookingTicket  `json:"tickets"`
	TotalPrice  int              `json:"price "`
}

type directusBookingTicket struct {
	ID    string `json:"id"`
	Price int    `json:"price"`
	QR    string `json:"qr"`
	Seat  Seat   `json:"seat_id"`
}

type directusBookingDetail struct {
	ID          string                   `json:"id"`
	BookingDate string                   `json:"date_created"`
	EventInfo   directusBookingEventInfo `json:"event_id"`
	Tickets     []directusBookingTicket  `json:"booking_items"`
	TotalPrice  int                      `json:"price "`
}

// GetBooking godoc
// @Summary      Get booking detail
// @Description  Retrieves the detailed information of a specific booking, including event details, schedules, and ticket data.
// @Tags         Bookings
// @Accept       json
// @Produce      json
// @Param        id             path      string  true   "Booking ID"
// @Success      200  {object}  BookingDetail     "Booking detail retrieved successfully"
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
	var directusResult directusBookingDetail
	statusCode, err := util.MakeRequest("GET", url, nil, token, &directusResult)
	if err != nil {
		util.LOGGER.Error("GET /api/bookings/:id: failed to get booking detail from Directus", "error", err, "id", id)
		ctx.JSON(statusCode, ErrorResponse{Message: err.Error()})
		return
	}

	// Build result object
	detail := BookingDetail{
		ID:          directusResult.ID,
		BookingDate: directusResult.BookingDate,
		EventInfo: BookingEventInfo{
			ID:           directusResult.EventInfo.ID,
			Name:         directusResult.EventInfo.Name,
			Address:      directusResult.EventInfo.Address,
			City:         directusResult.EventInfo.City,
			Country:      directusResult.EventInfo.Country,
			PreviewImage: util.CreateImageLink(directusResult.EventInfo.PreviewImage),
			Category:     directusResult.EventInfo.Category,
			Schedules:    directusResult.EventInfo.Schedules,
		},
		Tickets:    make([]BookingTicket, len(directusResult.Tickets)),
		TotalPrice: 0,
	}

	for i, ticket := range directusResult.Tickets {
		detail.Tickets[i] = BookingTicket{
			ID:    ticket.ID,
			Price: ticket.Price,
			QR:    util.CreateImageLink(ticket.QR),
			Seat:  ticket.Seat,
		}
		detail.TotalPrice += ticket.Price
	}

	ctx.JSON(http.StatusOK, detail)
}
