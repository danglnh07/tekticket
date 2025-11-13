package api

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"strings"
	"tekticket/db"
	"tekticket/util"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetEvent godoc
// @Summary      Retrieve a single event by ID or by its slug
// @Description  Returns detailed information about a specific event, including category, images, and schedule data.
// @Tags         Events
// @Accept       json
// @Produce      json
// @Param        id              path      string  true   "Event ID"
// @Success      200  {object}  db.Event            "Event details retrieved successfully"
// @Failure      400  {object}  ErrorResponse     "Invalid or missing event ID"
// @Failure      401  {object}  ErrorResponse     "Unauthorized access"
// @Failure      404  {object}  ErrorResponse     "Event not found or not published"
// @Failure      500  {object}  ErrorResponse     "Internal server error or failed to communicate with Directus"
// @Security BearerAuth
// @Router       /api/events/{id} [get]
func (server *Server) GetEvent(ctx *gin.Context) {
	// Get access token
	token := server.GetToken(ctx)

	// Get event ID by request path parameter
	id := ctx.Param("id") // Although it was called id, it can be either event ID or slug
	if id == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{Message: "Event ID is required"})
		return
	}

	// Build the query URL with status fields
	queryParams := url.Values{}
	fields := []string{
		"id", "name", "description", "address", "city", "country", "slug", "preview_image",
		"event_schedules.id", "event_schedules.start_time", "event_schedules.end_time",
		"event_schedules.start_checkin_time", "event_schedules.end_checkin_time",
		"seat_zones.id", "seat_zones.description", "seat_zones.total_seats", "seat_zones.status",
		"seat_zones.seats.id", "seat_zones.seats.status", "seat_zones.seats.seat_number",
		"tickets.id", "tickets.rank", "tickets.description", "tickets.base_price", "tickets.status",
		"tickets.ticket_selling_schedules.id", "tickets.ticket_selling_schedules.total",
		"tickets.ticket_selling_schedules.available", "tickets.ticket_selling_schedules.start_selling_time",
		"tickets.ticket_selling_schedules.end_selling_time", "tickets.ticket_selling_schedules.status",
		"creator_id.first_name", "creator_id.last_name", "creator_id.email",
		"category_id.id", "category_id.name", "category_id.description", "category_id.status",
	}
	queryParams.Add("fields", strings.Join(fields, ","))
	queryParams.Add("deep[seat_zones][_filter][status][_icontains]", "published")
	queryParams.Add("deep[tickets][_filter][status][_icontains]", "published")
	queryParams.Add("deep[tickets][deep][ticket_selling_schedules][_filter][status][_icontains]", "published")
	queryParams.Add("filter[category_id][status][_icontains]", "published")
	queryParams.Add("filter[status][_icontains]", "published")

	// Check if 'id' is an actual UUID (search by ID), or a normal string (search by slug)
	// If 'id' is an UUID, then we'll hit the single item endpoint (/items/events/:id), which is faster and cleaner
	// If 'id' is a string (slug), then we will have to search for every event that match this slug, and get the first item,
	// which is slower, but better SEO
	if _, err := uuid.Parse(id); err != nil {
		queryParams.Add("filter[slug][_icontains]", id)
		url := fmt.Sprintf("%s/items/events?%s", server.config.DirectusAddr, queryParams.Encode())

		// Make request to Directus
		var results []db.Event
		status, err := db.MakeRequest("GET", url, nil, token, &results)
		if err != nil {
			util.LOGGER.Error("GET /api/events/:id: failed to get event from Directus", "status", status, "error", err, "id", id)
			server.DirectusError(ctx, err)
			return
		}

		// If empty slice -> not found
		if len(results) == 0 {
			ctx.JSON(http.StatusNotFound, ErrorResponse{"No event found"})
			return
		}
		event := results[0]

		// Remap preview_image ID into a useable link
		if event.PreviewImage != "" {
			event.PreviewImage = util.CreateImageLink(server.config.ServerDomain, event.PreviewImage)
		}

		ctx.JSON(http.StatusOK, event)
	} else {
		url := fmt.Sprintf("%s/items/events/%s?%s", server.config.DirectusAddr, id, queryParams.Encode())
		var event db.Event
		status, err := db.MakeRequest("GET", url, nil, token, &event)
		if err != nil {
			util.LOGGER.Error("GET /api/events/:id: failed to get event from Directus", "status", status, "error", err, "id", id)
			server.DirectusError(ctx, err)
			return
		}

		// Remap preview_image ID into a useable link
		if event.PreviewImage != "" {
			event.PreviewImage = util.CreateImageLink(server.config.ServerDomain, event.PreviewImage)
		}

		ctx.JSON(http.StatusOK, event)
	}
}

// Helper method: calculate the smallest base price of a ticket belong to an event
func (server *Server) calculateEventMinimumBasePrice(tickets []db.Ticket) int {
	if len(tickets) == 0 {
		return 0
	}

	minPrice := tickets[0].BasePrice
	for i := 1; i < len(tickets); i++ {
		minPrice = min(minPrice, tickets[i].BasePrice)
	}

	return minPrice
}

// Helper method: get the nearest (before or after) start time of an event
func (server *Server) getNearestEventStartTime(schedules []db.EventSchedule) string {
	if len(schedules) == 0 {
		return ""
	}

	now := time.Now()
	nearestDiff := time.Duration(math.MaxInt64)
	var nearest *db.DateTime

	for _, schedule := range schedules {
		diff := now.Sub(time.Time(*schedule.StartTime)).Abs()
		if diff < nearestDiff {
			nearestDiff = diff
			nearest = schedule.StartTime
		}
	}

	return time.Time(*nearest).String()
}

// Event minimal info for list view
type EventInfo struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Address      string      `json:"address"`
	City         string      `json:"city"`
	Country      string      `json:"country"`
	PreviewImage string      `json:"preview_image"`
	Category     db.Category `json:"category"`
	StartTime    string      `json:"start_time"` // Closest upcoming schedule time
	BasePrice    int         `json:"base_price"` // Minimum ticket price
}

// ListEvents godoc
// @Summary      List all events
// @Description  Returns a list of published events with minimal information
// @Tags         Events
// @Accept       json
// @Produce      json
// @Param        name         query     string  false  "Filter by event name (case-insensitive contains)"
// @Param        location     query     string  false  "Filter by city or country (case-insensitive contains)"
// @Param        category     query     string  false  "Filter by category name (case-insensitive contains)"
// @Param        limit        query     int     false  "Limit number of results (default: 50)"
// @Param        offset       query     int     false  "Offset for pagination (default: 0)"
// @Param        sort         query     string  false  "Sort field (default: -date_created). Use - prefix for descending"
// @Success      200  {array}   EventInfo           "List of events retrieved successfully"
// @Failure      401  {object}  ErrorResponse       "Unauthorized access"
// @Failure      500  {object}  ErrorResponse       "Internal server error"
// @Security BearerAuth
// @Router       /api/events [get]
func (server *Server) ListEvents(ctx *gin.Context) {
	// Get access token
	token := server.GetToken(ctx)

	// Build query parameters
	queryParams := url.Values{}

	// Fields to retrieve
	fields := []string{
		"id", "status", "name", "address", "city", "country", "preview_image",
		"event_schedules.start_time",
		"tickets.base_price", "tickets.status",
		"category_id.id", "category_id.name", "category_id.description", "category_id.status",
	}
	queryParams.Add("fields", strings.Join(fields, ","))

	// Filter: only published events
	queryParams.Add("filter[status][_eq]", "published")

	// Filter: only fetch category that is published
	queryParams.Add("deep[category_id][_filter][status][_icontains]", "published")

	// Filter: by name (case-insensitive)
	if name := ctx.Query("name"); name != "" {
		queryParams.Add("filter[name][_icontains]", name)
	}

	// Filter: by location (city OR country)
	if location := ctx.Query("location"); location != "" {
		queryParams.Add("filter[_or][0][city][_icontains]", location)
		queryParams.Add("filter[_or][1][country][_icontains]", location)
	}

	// Filter: by category name
	if category := ctx.Query("category"); category != "" {
		queryParams.Add("filter[category_id][name][_icontains]", category)
	}

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

	// Build URL
	directusURL := fmt.Sprintf("%s/items/events?%s", server.config.DirectusAddr, queryParams.Encode())

	// Make request to Directus
	var directusResult []db.Event
	status, err := db.MakeRequest("GET", directusURL, nil, token, &directusResult)
	if err != nil {
		util.LOGGER.Error("GET /api/events: failed to get events from Directus", "status", status, "error", err)
		server.DirectusError(ctx, err)
		return
	}

	// Transform and filter data
	events := make([]EventInfo, 0)

	for _, event := range directusResult {
		// Create event info
		eventInfo := EventInfo{
			ID:           event.ID,
			Name:         event.Name,
			Address:      event.Address,
			City:         event.City,
			Country:      event.Country,
			PreviewImage: event.PreviewImage,
			Category:     *event.Category,
		}

		// Calculate smallest base price for this event
		eventInfo.BasePrice = server.calculateEventMinimumBasePrice(event.Tickets)

		// Get the nearest time in relative to the current time
		eventInfo.StartTime = server.getNearestEventStartTime(event.EventSchedules)

		// Remap preview_image ID to link
		if eventInfo.PreviewImage != "" {
			eventInfo.PreviewImage = util.CreateImageLink(server.config.ServerDomain, eventInfo.PreviewImage)
		}

		events = append(events, eventInfo)
	}

	// Return empty array if no events found
	ctx.JSON(http.StatusOK, events)
}

// GetCategories godoc
// @Summary      Retrieve all categories
// @Description  Returns a list of all available event categories from the database.
// @Tags         Events
// @Accept       json
// @Produce      json
// @Success      200  {array}  db.Category "List of categories retrieved successfully"
// @Failure      401  {object}  ErrorResponse         "Unauthorized access"
// @Failure      500  {object}  ErrorResponse         "Internal server error"
// @Security BearerAuth
// @Router       /api/categories [get]
func (server *Server) GetCategories(ctx *gin.Context) {
	// Get access token
	token := server.GetToken(ctx)
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Unauthorized access"})
		return
	}

	// Build the query URL
	queryParams := url.Values{}
	queryParams.Add("fields", "id,name,description")
	queryParams.Add("sort", "name")
	queryParams.Add("limit", "-1")
	queryParams.Add("filter[status][_icontains]", "published")

	directusURL := fmt.Sprintf("%s/items/categories?%s", server.config.DirectusAddr, queryParams.Encode())

	// Make request to Directus
	var categories []db.Category
	status, err := db.MakeRequest("GET", directusURL, nil, token, &categories)
	if err != nil {
		util.LOGGER.Error("GET /api/events/categories: failed to get categories from Directus", "status", status, "error", err)
		server.DirectusError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, categories)
}
