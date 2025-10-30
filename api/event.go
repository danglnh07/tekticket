package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"tekticket/util"

	"github.com/gin-gonic/gin"
)

// EventListResponse represents the response for list events
type EventListResponse struct {
	Data []Event `json:"data"`
	Meta *Meta   `json:"meta,omitempty"`
}

// Meta represents pagination metadata
type Meta struct {
	TotalCount  int `json:"total_count"`
	FilterCount int `json:"filter_count"`
	Limit       int `json:"limit"`
	Offset      int `json:"offset"`
}

// Event struct
type Event struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Location    string   `json:"location"`
	Category    string   `json:"category"`
	StartTime   string   `json:"start_time"`
	EndTime     string   `json:"end_time"`
	Image       string   `json:"image,omitempty"`
	Price       *float64 `json:"price,omitempty"`
	Status      string   `json:"status"`
	DateCreated string   `json:"date_created"`
	Organizer   string   `json:"organizer,omitempty"`
	Capacity    *int     `json:"capacity,omitempty"`
	TicketsSold *int     `json:"tickets_sold,omitempty"`
}

// GetEvents godoc
// @Summary      Retrieve list of events
// @Description  Returns a paginated list of events with optional filters for search, category, location, date, and status.
// @Description  If `chose_date` is provided, only events that are active during that date will be returned.
// @Tags         Events
// @Accept       json
// @Produce      json
// @Param        search          query     string  false  "Search by event title or description"
// @Param        category        query     string  false  "Filter by category ID"
// @Param        location        query     string  false  "Filter by location"
// @Param        chose_date      query     string  false  "Filter events active on this date (YYYY-MM-DD or ISO format)"
// @Param        status          query     string  false  "Filter by status (default: published)"
// @Param        sort            query     string  false  "Sort field (default: -date_created)"
// @Param        limit           query     int     false  "Limit number of items (default: 20, max: 100)"
// @Param        offset          query     int     false  "Offset for pagination (default: 0)"
// @Success      200  {object}  EventListResponse  "List of events retrieved successfully"
// @Failure      400  {object}  ErrorResponse       "Invalid query parameters"
// @Failure      401  {object}  ErrorResponse       "Unauthorized access"
// @Failure      500  {object}  ErrorResponse       "Internal server error or Directus failure"
// @Router       /api/events [get]
func (server *Server) GetEvents(ctx *gin.Context) {
	// Get user access token
	token := server.GetToken(ctx)
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Unauthorized access"})
		return
	}

	// Get filter parameters
	search := ctx.Query("search")
	category := ctx.Query("category")
	location := ctx.Query("location")
	choseDate := ctx.Query("chose_date")
	status := ctx.DefaultQuery("status", "published")
	sortField := ctx.DefaultQuery("sort", "-date_created")

	limit, err := strconv.Atoi(ctx.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 {
		util.LOGGER.Warn("GET /api/events: invalid limit parameter, defaulting to 20", "input", ctx.Query("limit"))
		limit = 20
	}
	if limit > 100 {
		util.LOGGER.Warn("GET /api/events: limit parameter exceed 100, defaulting to 100", "limit", limit)
		limit = 100
	}

	offset, err := strconv.Atoi(ctx.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		util.LOGGER.Warn("GET /api/events: offset parameter invalid, defaulting to 0", "offset", ctx.Query("offset"))
		offset = 0
	}

	// normalize choose_date (if user only provide YYYY-MM-DD, without the time segments)
	normalizedChoseDate := util.NormalizeChoseDate(choseDate)
	util.LOGGER.Info("GET /api/events: date normalization", "original", choseDate, "normalized", normalizedChoseDate)

	// If choose_date is provided, we only fetch events that has their schedule include choose_date
	var eventIDs []string
	if normalizedChoseDate != "" {
		// start_time <= choose_date <= end_time
		filterStr := fmt.Sprintf(
			`{"_and":[{"start_time":{"_lte":"%s"}},{"end_time":{"_gte":"%s"}}]}`,
			normalizedChoseDate, normalizedChoseDate,
		)
		util.LOGGER.Info("GET /api/events: time filter", "filter_str", filterStr)

		// Build the query URL
		scheduleURL := fmt.Sprintf(
			"%s/items/event_schedules?filter=%s&fields=event_id.id&limit=-1",
			server.config.DirectusAddr,
			url.QueryEscape(filterStr),
		)

		// Make request to Directus
		scheduleResp, statusCode, err := util.MakeRequest("GET", scheduleURL, nil, token)
		if err != nil {
			util.LOGGER.Error("GET /api/events: failed to get schedules from Directus", "error", err)
			ctx.JSON(statusCode, ErrorResponse{Message: err.Error()})
			return
		}
		defer scheduleResp.Body.Close()

		// Parse response from Directus request
		var scheduleDirectusResp struct {
			Data []struct {
				EventID struct {
					ID string `json:"id"`
				} `json:"event_id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(scheduleResp.Body).Decode(&scheduleDirectusResp); err != nil {
			util.LOGGER.Error("GET /api/events: failed to decode schedules response", "error", err)
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Internal server error"})
			return
		}

		// 1 event can have different opening time, but these range-time will never overlapped, so there would be no way
		// that an event ID can be duplicate
		for _, row := range scheduleDirectusResp.Data {
			eventIDs = append(eventIDs, row.EventID.ID)
		}
	}

	// If filter date param provided, and no eventID found, then we stop and return here
	if normalizedChoseDate != "" && len(eventIDs) == 0 {
		util.LOGGER.Info("GET /api/events: no events found matching time filter", "chose_date", normalizedChoseDate)
		ctx.JSON(http.StatusOK, EventListResponse{
			Data: []Event{},
			Meta: &Meta{TotalCount: 0, FilterCount: 0, Limit: limit, Offset: offset},
		})
		return
	}

	// Build filter to get the list of events
	filters := BuildEventFilters(search, category, location, status)
	if len(eventIDs) > 0 {
		idFilter := fmt.Sprintf(`{"id":{"_in":["%s"]}}`, strings.Join(eventIDs, `","`))
		if filters == "" {
			filters = idFilter
		} else {
			filters = fmt.Sprintf(`{"_and":[%s,%s]}`, filters, idFilter)
		}
	}

	// Make API to Directus to get events
	queryParams := url.Values{}
	if filters != "" {
		queryParams.Add("filter", filters)
	}

	queryParams.Add("fields", "*,category_id.id,category_id.name,image.id,preview_image.id")
	queryParams.Add("limit", strconv.Itoa(limit))
	queryParams.Add("offset", strconv.Itoa(offset))
	queryParams.Add("sort", sortField)
	queryParams.Add("meta", "total_count,filter_count")

	eventURL := fmt.Sprintf("%s/items/events?%s", server.config.DirectusAddr, queryParams.Encode())
	eventResp, statusCode, err := util.MakeRequest("GET", eventURL, nil, token)
	if err != nil {
		util.LOGGER.Error("GET /api/events: failed to get events from Directus", "error", err)
		ctx.JSON(statusCode, ErrorResponse{Message: err.Error()})
		return
	}
	defer eventResp.Body.Close()

	// Parse events response from Directus
	var directusEventResp struct {
		Data []map[string]any `json:"data"`
		Meta *struct {
			TotalCount  int `json:"total_count"`
			FilterCount int `json:"filter_count"`
		} `json:"meta,omitempty"`
	}

	if err := json.NewDecoder(eventResp.Body).Decode(&directusEventResp); err != nil {
		util.LOGGER.Error("GET /api/events: failed to decode Directus response", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Internal server error"})
		return
	}

	// Map result
	events := make([]Event, 0, len(directusEventResp.Data))
	for _, item := range directusEventResp.Data {
		event := MapToEvent(item)
		if event.Image != "" {
			event.Image = util.CreateImageLink(event.Image)
		}
		events = append(events, event)
	}

	// If choose_date filter provided, we need to attach additional information to the response
	events = AttachScheduleToEvents(
		events,
		normalizedChoseDate,
		"", // ed (end-date filter) if future needed
		server.config.DirectusAddr,
		server.config.DirectusStaticToken,
	)

	response := EventListResponse{Data: events}
	if directusEventResp.Meta != nil {
		response.Meta = &Meta{
			TotalCount:  directusEventResp.Meta.TotalCount,
			FilterCount: directusEventResp.Meta.FilterCount,
			Limit:       limit,
			Offset:      offset,
		}
	}

	ctx.JSON(http.StatusOK, response)
}

// GetEventByID godoc
// @Summary      Retrieve a single event by ID
// @Description  Returns detailed information about a specific event, including category, images, and schedule data.
// @Tags         Events
// @Accept       json
// @Produce      json
// @Param        id              path      string  true   "Event ID"
// @Success      200  {object}  Event             "Event details retrieved successfully"
// @Failure      400  {object}  ErrorResponse     "Invalid or missing event ID"
// @Failure      401  {object}  ErrorResponse     "Unauthorized access"
// @Failure      500  {object}  ErrorResponse     "Internal server error or failed to communicate with Directus"
// @Router       /api/events/{id} [get]
func (server *Server) GetEventByID(ctx *gin.Context) {
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

	// Build the query URL
	queryParams := url.Values{}
	queryParams.Add("fields", "*,category_id.id,category_id.name,image.id,preview_image.id")
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
		Data map[string]any `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
		util.LOGGER.Error("GET /api/events/:id: failed to decode Directus response", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Internal server error"})
		return
	}

	event := MapToEvent(directusResp.Data)

	// Since this is single GET, we don't have a date filter, so we pass empty value to get the available one
	events := AttachScheduleToEvents(
		[]Event{event},
		"",
		"",
		server.config.DirectusAddr,
		server.config.DirectusStaticToken,
	)
	if len(events) > 0 {
		event = events[0]
	}
	if event.Image != "" {
		event.Image = util.CreateImageLink(event.Image)
	}

	ctx.JSON(http.StatusOK, event)
}

// Helper method: build the filter query string for event querying
func BuildEventFilters(search, category, location, status string) string {
	var filters []string

	if status != "" {
		filters = append(filters, fmt.Sprintf(`{"status":{"_eq":"%s"}}`, status))
	}
	if search != "" {
		searchFilter := fmt.Sprintf(`{"_or":[{"name":{"_icontains":"%s"}},{"description":{"_icontains":"%s"}}]}`, search, search)
		filters = append(filters, searchFilter)
	}
	if category != "" {
		categoryFilter := fmt.Sprintf(`{"category_id":{"_eq":"%s"}}`, category)
		filters = append(filters, categoryFilter)
	}
	if location != "" {
		filters = append(filters, fmt.Sprintf(`{"city":{"_icontains":"%s"}}`, location))
	}

	if len(filters) == 0 {
		return ""
	}
	if len(filters) == 1 {
		return filters[0]
	}
	return fmt.Sprintf(`{"_and":[%s]}`, strings.Join(filters, ","))
}

// Map response map into Event struct
func MapToEvent(data map[string]any) Event {
	event := Event{}

	if id, ok := data["id"].(string); ok {
		event.ID = id
	}
	if name, ok := data["name"].(string); ok {
		event.Name = name
	}
	if description, ok := data["description"].(string); ok {
		event.Description = description
	}

	// Location = Address + City + Country
	address := ""
	city := ""
	country := ""
	if a, ok := data["address"].(string); ok {
		address = a
	}
	if c, ok := data["city"].(string); ok {
		city = c
	}
	if co, ok := data["country"].(string); ok {
		country = co
	}
	parts := []string{}
	for _, p := range []string{address, city, country} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	event.Location = strings.Join(parts, ", ")

	// Handle category
	if catObj, ok := data["category_id"].(map[string]any); ok {
		if catName, ok := catObj["name"].(string); ok && catName != "" {
			event.Category = catName
		} else if catID, ok := catObj["id"].(string); ok {
			event.Category = catID
		}
	} else if catStr, ok := data["category_id"].(string); ok {
		event.Category = catStr
	}

	if previewImage, ok := data["preview_image"].(string); ok {
		event.Image = util.CreateImageLink(previewImage)
	}

	if startTime, ok := data["start_time"].(string); ok {
		event.StartTime = startTime
	}
	if endTime, ok := data["end_time"].(string); ok {
		event.EndTime = endTime
	}

	if price, ok := data["price"].(float64); ok {
		event.Price = &price
	}
	if status, ok := data["status"].(string); ok {
		event.Status = status
	}
	if dateCreated, ok := data["date_created"].(string); ok {
		event.DateCreated = dateCreated
	}
	if organizer, ok := data["organizer"].(string); ok {
		event.Organizer = organizer
	}
	if capacity, ok := data["capacity"].(float64); ok {
		cap := int(capacity)
		event.Capacity = &cap
	}
	if ticketsSold, ok := data["tickets_sold"].(float64); ok {
		sold := int(ticketsSold)
		event.TicketsSold = &sold
	}

	return event
}

// Attach start/end time from event_schedules to each event
func AttachScheduleToEvents(events []Event, choseDate, ed, directusAddr, token string) []Event {
	if len(events) == 0 {
		return events
	}

	ids := make([]string, 0, len(events))
	for _, e := range events {
		ids = append(ids, e.ID)
	}

	idFilters := make([]string, 0, len(ids))
	for _, id := range ids {
		idFilters = append(idFilters, fmt.Sprintf(`{"event_id":{"_eq":"%s"}}`, id))
	}
	base := fmt.Sprintf(`{"_or":[%s]}`, strings.Join(idFilters, ","))

	var timeFilter string
	if choseDate != "" {
		// Logic: (start_time <= choseDate) AND (end_time >= choseDate)
		timeFilter = fmt.Sprintf(`{"_and":[{"start_time":{"_lte":"%s"}},{"end_time":{"_gte":"%s"}}]}`, choseDate, choseDate)
	}

	var filter string
	if timeFilter != "" {
		filter = fmt.Sprintf(`{"_and":[%s,%s]}`, base, timeFilter)
	} else {
		filter = base
	}

	qp := url.Values{}
	qp.Add("filter", filter)
	qp.Add("fields", "event_id,start_time,end_time")
	qp.Add("sort", "start_time") // Get the earliest
	qp.Add("limit", "-1")        // If Directus allow unlimited

	u := fmt.Sprintf("%s/items/event_schedules?%s", directusAddr, qp.Encode())
	resp, _, err := util.MakeRequest("GET", u, nil, token)
	if err != nil || resp == nil {
		util.LOGGER.Error("AttachScheduleToEvents: failed to get schedules", "error", err)
		return events
	}
	defer resp.Body.Close()

	var payload struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		util.LOGGER.Error("AttachScheduleToEvents: decode error", "error", err)
		return events
	}

	// Group in event ID
	scheduleMap := make(map[string][]map[string]any)
	for _, s := range payload.Data {
		if id, ok := s["event_id"].(string); ok {
			scheduleMap[id] = append(scheduleMap[id], s)
			continue
		}
		if obj, ok := s["event_id"].(map[string]any); ok {
			if id, ok := obj["id"].(string); ok {
				scheduleMap[id] = append(scheduleMap[id], s)
			}
		}
	}

	// Attach start/end time from schedules
	for i, e := range events {
		if list, ok := scheduleMap[e.ID]; ok && len(list) > 0 {
			// Since it's sorted, list[0] is the earliest in range
			if st, ok := list[0]["start_time"].(string); ok {
				events[i].StartTime = st
			}
			if et, ok := list[0]["end_time"].(string); ok {
				events[i].EndTime = et
			}
		} else {
			// No schedule match, leave empty
			if choseDate != "" {
				events[i].StartTime = ""
				events[i].EndTime = ""
			}
		}
	}

	return events
}

// Category represents a category from the database
type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CategoryListResponse represents the response for list categories
type CategoryListResponse struct {
	Data []Category `json:"data"`
}

// GetCategories godoc
// @Summary      Retrieve all categories
// @Description  Returns a list of all available event categories from the database.
// @Tags         Events
// @Accept       json
// @Produce      json
// @Success      200  {object}  CategoryListResponse  "List of categories retrieved successfully"
// @Failure      401  {object}  ErrorResponse         "Unauthorized access"
// @Failure      500  {object}  ErrorResponse         "Internal server error"
// @Router       /api/events/categories [get]
func (server *Server) GetCategories(ctx *gin.Context) {
	// Get access token
	token := server.GetToken(ctx)
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Unauthorized access"})
		return
	}

	// Build the query URL
	queryParams := url.Values{}
	queryParams.Add("fields", "id,name")
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
		Data []map[string]any `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
		util.LOGGER.Error("GET /api/events/categories: failed to decode Directus response", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Internal server error"})
		return
	}

	// Map result
	categories := make([]Category, 0, len(directusResp.Data))
	for _, item := range directusResp.Data {
		category := Category{}
		if id, ok := item["id"].(string); ok {
			category.ID = id
		}
		if name, ok := item["name"].(string); ok {
			category.Name = name
		}
		categories = append(categories, category)
	}

	ctx.JSON(http.StatusOK, CategoryListResponse{Data: categories})
}
