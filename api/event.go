package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"strings"
	"tekticket/util"

	"github.com/gin-gonic/gin"
)

// Event schedule data returned
type EventSchedule struct {
	ID               string `json:"id"`
	StartTime        string `json:"start_time"`
	EndTime          string `json:"end_time"`
	StartCheckinTime string `json:"start_checkin_time"`
	EndCheckinTime   string `json:"end_checkin_time"`
}

// Seat data returned
type Seat struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	SeatNumber string `json:"seat_number"`
}

// Seat zone data returned
type SeatZone struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	TotalSeats  int    `json:"total_seats"`
	Seats       []Seat `json:"seats"`
}

// Ticket selling schedules data returned
type TicketSellingSchedule struct {
	ID               string `json:"id"`
	Total            int    `json:"total"`
	Available        int    `json:"available"`
	StartSellingTime string `json:"start_selling_time"`
	EndSellingTime   string `json:"end_selling_time"`
}

// Ticket data returned
type Ticket struct {
	ID                     string                  `json:"id"`
	Rank                   string                  `json:"rank"`
	Description            string                  `json:"description"`
	BasePrice              int                     `json:"base_price"`
	TicketSellingSchedules []TicketSellingSchedule `json:"ticket_selling_schedules"`
}

// Event creator data returned
type Creator struct {
	Firstname string `json:"first_name"`
	Lastname  string `json:"last_name"`
	Email     string `json:"email"`
}

// Category data returned
type Category struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Event data return
type EventDetail struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Address        string          `json:"address"`
	City           string          `json:"city"`
	Country        string          `json:"country"`
	Slug           string          `json:"slug"`
	PreviewImage   string          `json:"preview_image"`
	EventSchedules []EventSchedule `json:"event_schedules"`
	SeatZones      []SeatZone      `json:"seat_zones"`
	Tickets        []Ticket        `json:"tickets"`
	Creator        Creator         `json:"creator"`
	Category       Category        `json:"categories"`
}

// Directus response structures with status fields
type directusSeatZone struct {
	SeatZone
	Status string `json:"status"`
}

type directusTicket struct {
	Ticket
	Status string `json:"status"`
}

type directusCategory struct {
	Category
	Status string `json:"status"`
}

type directusEventDetail struct {
	ID             string             `json:"id"`
	Status         string             `json:"status"`
	Name           string             `json:"name"`
	Description    string             `json:"description"`
	Address        string             `json:"address"`
	City           string             `json:"city"`
	Country        string             `json:"country"`
	Slug           string             `json:"slug"`
	PreviewImage   string             `json:"preview_image"`
	EventSchedules []EventSchedule    `json:"event_schedules"`
	SeatZones      []directusSeatZone `json:"seat_zones"`
	Tickets        []directusTicket   `json:"tickets"`
	Creator        Creator            `json:"creator_id"`
	Category       directusCategory   `json:"category_id"`
}

// GetEventByID godoc
// @Summary      Retrieve a single event by ID
// @Description  Returns detailed information about a specific event, including category, images, and schedule data.
// @Tags         Events
// @Accept       json
// @Produce      json
// @Param        id              path      string  true   "Event ID"
// @Success      200  {object}  EventDetail             "Event details retrieved successfully"
// @Failure      400  {object}  ErrorResponse     "Invalid or missing event ID"
// @Failure      401  {object}  ErrorResponse     "Unauthorized access"
// @Failure      404  {object}  ErrorResponse     "Event not found or not published"
// @Failure      500  {object}  ErrorResponse     "Internal server error or failed to communicate with Directus"
// @Security BearerAuth
// @Router       /api/events/{id} [get]
func (server *Server) GetEvent(ctx *gin.Context) {
	// Get access token
	token := server.GetToken(ctx)
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Unauthorized access"})
		return
	}

	// Get event ID by request path parameter
	id := ctx.Param("id")
	if id == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{Message: "Event ID is required"})
		return
	}

	// Build the query URL with status fields
	queryParams := url.Values{}
	fields := []string{
		"*",
		"event_schedules.id", "event_schedules.start_time", "event_schedules.end_time",
		"event_schedules.start_checkin_time", "event_schedules.end_checkin_time",
		"seat_zones.id", "seat_zones.description", "seat_zones.total_seats", "seat_zones.status",
		"seat_zones.seats.id", "seat_zones.seats.status", "seat_zones.seats.seat_number",
		"tickets.id", "tickets.rank", "tickets.description", "tickets.base_price", "tickets.status",
		"creator_id.first_name", "creator_id.last_name", "creator_id.email",
		"category_id.id", "category_id.name", "category_id.description", "category_id.status",
	}
	queryParams.Add("fields", strings.Join(fields, ","))
	directusURL := fmt.Sprintf("%s/items/events/%s?%s", server.config.DirectusAddr, id, queryParams.Encode())

	// Make request to Directus
	resp, statusCode, err := util.MakeRequest("GET", directusURL, nil, token)
	if err != nil {
		util.LOGGER.Error("GET /api/events/:id: failed to get event from Directus", "error", err, "id", id)
		ctx.JSON(statusCode, ErrorResponse{Message: err.Error()})
		return
	}
	defer resp.Body.Close()

	// Parse response from Directus request
	var directusResp struct {
		Data directusEventDetail `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
		util.LOGGER.Error("GET /api/events/:id: failed to decode Directus response", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Internal server error"})
		return
	}

	// Check if event is published
	if directusResp.Data.Status != "published" {
		ctx.JSON(http.StatusNotFound, ErrorResponse{Message: "Event not found"})
		return
	}

	// Filter and transform the data
	eventDetail := EventDetail{
		ID:             directusResp.Data.ID,
		Name:           directusResp.Data.Name,
		Description:    directusResp.Data.Description,
		Address:        directusResp.Data.Address,
		City:           directusResp.Data.City,
		Country:        directusResp.Data.Country,
		Slug:           directusResp.Data.Slug,
		PreviewImage:   directusResp.Data.PreviewImage,
		Creator:        directusResp.Data.Creator,
		EventSchedules: directusResp.Data.EventSchedules,
		SeatZones:      make([]SeatZone, 0),
		Tickets:        make([]Ticket, 0),
	}

	// Filter seat_zones - only published, and initialize seats array
	for _, zone := range directusResp.Data.SeatZones {
		if zone.Status == "published" {
			seatZone := zone.SeatZone
			// Ensure seats is empty array, not null
			if seatZone.Seats == nil {
				seatZone.Seats = make([]Seat, 0)
			}
			eventDetail.SeatZones = append(eventDetail.SeatZones, seatZone)
		}
	}

	// Filter tickets - only published, and initialize ticket_selling_schedules array
	for _, ticket := range directusResp.Data.Tickets {
		if ticket.Status == "published" {
			t := ticket.Ticket
			// Ensure ticket_selling_schedules is empty array, not null
			if t.TicketSellingSchedules == nil {
				t.TicketSellingSchedules = make([]TicketSellingSchedule, 0)
			}
			eventDetail.Tickets = append(eventDetail.Tickets, t)
		}
	}

	// Handle category - only if published, otherwise return empty struct
	if directusResp.Data.Category.Status == "published" {
		eventDetail.Category = directusResp.Data.Category.Category
	} else {
		eventDetail.Category = Category{}
	}

	// Remap preview_image ID into a useable link
	if eventDetail.PreviewImage != "" {
		eventDetail.PreviewImage = util.CreateImageLink(eventDetail.PreviewImage)
	}

	ctx.JSON(http.StatusOK, eventDetail)
}

// Event minimal info for list view
type EventInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Address      string   `json:"address"`
	City         string   `json:"city"`
	Country      string   `json:"country"`
	PreviewImage string   `json:"preview_image"`
	Category     Category `json:"category"`
	StartTime    string   `json:"start_time"` // Closest upcoming schedule time
	BasePrice    int      `json:"base_price"` // Minimum ticket price
}

// Directus response structures for list events
type directusEventScheduleMinimal struct {
	StartTime string `json:"start_time"`
}

type directusTicketMinimal struct {
	BasePrice int    `json:"base_price"`
	Status    string `json:"status"`
}

type directusEventInfo struct {
	ID             string                         `json:"id"`
	Status         string                         `json:"status"`
	Name           string                         `json:"name"`
	Address        string                         `json:"address"`
	City           string                         `json:"city"`
	Country        string                         `json:"country"`
	PreviewImage   string                         `json:"preview_image"`
	EventSchedules []directusEventScheduleMinimal `json:"event_schedules"`
	Tickets        []directusTicketMinimal        `json:"tickets"`
	Category       directusCategory               `json:"category_id"`
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
// @Param        min_price    query     int     false  "Filter by minimum base price"
// @Param        max_price    query     int     false  "Filter by maximum base price"
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
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Unauthorized access"})
		return
	}

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

	// Filter: by name (case-insensitive)
	if name := ctx.Query("name"); name != "" {
		queryParams.Add("filter[name][_icontains]", name)
	}

	// Filter: by location (city OR country)
	if location := ctx.Query("location"); location != "" {
		// Use JSON filter for OR condition
		locationFilter := fmt.Sprintf(`{"_or":[{"city":{"_icontains":"%s"}},{"country":{"_icontains":"%s"}}]}`,
			location, location)
		queryParams.Add("filter", locationFilter)
	}

	// Filter: by category name
	if category := ctx.Query("category"); category != "" {
		queryParams.Add("filter[category_id][name][_icontains]", category)
	}

	// Note: Price filtering is done post-fetch since we need to calculate min price from tickets
	minPrice := 0
	maxPrice := 0
	if minPriceStr := ctx.Query("min_price"); minPriceStr != "" {
		if val, err := strconv.Atoi(minPriceStr); err == nil {
			minPrice = val
		}
	}
	if maxPriceStr := ctx.Query("max_price"); maxPriceStr != "" {
		if val, err := strconv.Atoi(maxPriceStr); err == nil {
			maxPrice = val
		}
	}

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

	// Build URL
	directusURL := fmt.Sprintf("%s/items/events?%s", server.config.DirectusAddr, queryParams.Encode())

	// Make request to Directus
	resp, statusCode, err := util.MakeRequest("GET", directusURL, nil, token)
	if err != nil {
		util.LOGGER.Error("GET /api/events: failed to get events from Directus", "error", err)
		ctx.JSON(statusCode, ErrorResponse{Message: err.Error()})
		return
	}
	defer resp.Body.Close()

	// Parse response
	var directusResp struct {
		Data []directusEventInfo `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
		util.LOGGER.Error("GET /api/events: failed to decode Directus response", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Internal server error"})
		return
	}

	// Transform and filter data
	events := make([]EventInfo, 0)

	for _, event := range directusResp.Data {
		// Skip if event is not published (redundant check)
		if event.Status != "published" {
			continue
		}

		// Calculate base price (minimum of published tickets)
		basePrice := 0
		hasPublishedTickets := false
		for _, ticket := range event.Tickets {
			if ticket.Status == "published" {
				if !hasPublishedTickets || ticket.BasePrice < basePrice {
					basePrice = ticket.BasePrice
					hasPublishedTickets = true
				}
			}
		}

		// Apply price filters
		if minPrice > 0 && basePrice < minPrice {
			continue
		}
		if maxPrice > 0 && basePrice > maxPrice {
			continue
		}

		// Find closest start time
		startTime := ""
		if len(event.EventSchedules) > 0 {
			currentTime := time.Now()
			var closestTime time.Time
			foundFutureTime := false

			for _, schedule := range event.EventSchedules {
				// Try multiple time formats
				var scheduleTime time.Time
				var err error

				// Try RFC3339 first (with timezone)
				scheduleTime, err = time.Parse(time.RFC3339, schedule.StartTime)
				if err != nil {
					// Try without timezone (assume UTC)
					scheduleTime, err = time.Parse("2006-01-02T15:04:05", schedule.StartTime)
					if err != nil {
						util.LOGGER.Warn("GET /api/events: failed to parse schedule start time",
							"time", schedule, "error", err)
						continue
					}
				}

				// Find the closest future time, or the latest past time if no future times
				if scheduleTime.After(currentTime) {
					util.LOGGER.Info("GET /api/events: schedule time after current time")
					if !foundFutureTime || scheduleTime.Before(closestTime) {
						util.LOGGER.Info("Not found future time or schedule time before closest time")
						closestTime = scheduleTime
						foundFutureTime = true
					}
				} else if !foundFutureTime {
					util.LOGGER.Info("GET /api/events: not found future time")
					if closestTime.IsZero() || scheduleTime.After(closestTime) {
						util.LOGGER.Info("GET /api/events: closest time is zero or schedule time after closest time")
						closestTime = scheduleTime
					}
				}
			}

			if !closestTime.IsZero() {
				util.LOGGER.Info("GET /api/events: closest time not zero")
				startTime = closestTime.Format(time.RFC3339)
			}
		}

		// Build category (only if published)
		category := Category{}
		if event.Category.Status == "published" {
			category = event.Category.Category
		}

		// Create event info
		eventInfo := EventInfo{
			ID:           event.ID,
			Name:         event.Name,
			Address:      event.Address,
			City:         event.City,
			Country:      event.Country,
			PreviewImage: event.PreviewImage,
			Category:     category,
			StartTime:    startTime,
			BasePrice:    basePrice,
		}

		// Remap preview_image ID to link
		if eventInfo.PreviewImage != "" {
			eventInfo.PreviewImage = util.CreateImageLink(eventInfo.PreviewImage)
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
// @Success      200  {array}  Category "List of categories retrieved successfully"
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

	directusURL := fmt.Sprintf("%s/items/categories?%s", server.config.DirectusAddr, queryParams.Encode())

	// Make request to Directus
	resp, statusCode, err := util.MakeRequest("GET", directusURL, nil, token)
	if err != nil {
		util.LOGGER.Error("GET /api/events/categories: failed to get categories from Directus", "error", err)
		ctx.JSON(statusCode, ErrorResponse{Message: err.Error()})
		return
	}
	defer resp.Body.Close()

	// Parse response from Directus request
	var directusResp struct {
		Data []Category `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
		util.LOGGER.Error("GET /api/events/categories: failed to decode Directus response", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Internal server error"})
		return
	}

	ctx.JSON(http.StatusOK, directusResp.Data)
}
