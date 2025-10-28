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

// =======================
// Models & DTOs
// =======================

// EventListResponse represents the response for list events
type EventListResponse struct {
	Data []Event `json:"data"`
	Meta *Meta   `json:"meta,omitempty"`
}

// EventDetailResponse represents the response for event detail
type EventDetailResponse struct {
	Data Event `json:"data"`
}

// Meta represents pagination metadata
type Meta struct {
	TotalCount  int `json:"total_count"`
	FilterCount int `json:"filter_count"`
	Limit       int `json:"limit"`
	Offset      int `json:"offset"`
}

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

// =======================
// Handlers
// =======================

// GET /api/events
func (server *Server) GetEvents(ctx *gin.Context) {
	search := ctx.Query("search")
	category := ctx.Query("category")
	location := ctx.Query("location")
	choseDate := ctx.Query("chose_date")
	status := ctx.DefaultQuery("status", "published")
	sortField := ctx.DefaultQuery("sort", "-date_created")

	limit, err := strconv.Atoi(ctx.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	offset, err := strconv.Atoi(ctx.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}

	// ---- Chuẩn hoá ngày chose_date (nếu user chỉ nhập YYYY-MM-DD) ----
	normalizeChoseDate := func(d string) string {
		if d == "" {
			return ""
		}
		if strings.Contains(d, "T") { // đã là ISO đầy đủ
			return d
		}
		// Chuyển thành đầu ngày UTC (tùy hệ thống có thể muốn kèm cả cuối ngày)
		return d + "T00:00:00Z"
	}
	normalizedChoseDate := normalizeChoseDate(choseDate)

	// Debug
	util.LOGGER.Info("Date normalization",
		"original_chose_date", choseDate,
		"normalized_chose_date", normalizedChoseDate,
	)

	// =========================================
	// Lấy danh sách event_id từ bảng event_schedules nếu có filter thời gian
	// =========================================
	var eventIDs []string
	util.LOGGER.Info("Time filter check",
		"chose_date", normalizedChoseDate,
		"has_time_filter", normalizedChoseDate != "",
	)
	if normalizedChoseDate != "" {
		// (start_time <= chose_date) AND (end_time >= chose_date)
		filterStr := fmt.Sprintf(
			`{"_and":[{"start_time":{"_lte":"%s"}},{"end_time":{"_gte":"%s"}}]}`,
			normalizedChoseDate, normalizedChoseDate,
		)

		util.LOGGER.Info("Time filter", "filter_str", filterStr)

		scheduleURL := fmt.Sprintf(
			"%s/items/event_schedules?filter=%s&fields=event_id.id&limit=-1",
			server.config.DirectusAddr,
			url.QueryEscape(filterStr),
		)

		resp1, _, err1 := util.MakeRequest("GET", scheduleURL, nil, server.config.DirectusStaticToken)
		if err1 == nil && resp1 != nil {
			defer resp1.Body.Close()
			var schRes struct {
				Data []struct {
					EventID struct {
						ID string `json:"id"`
					} `json:"event_id"`
				} `json:"data"`
			}
			if err := json.NewDecoder(resp1.Body).Decode(&schRes); err == nil {
				seen := make(map[string]struct{})
				for _, row := range schRes.Data {
					id := row.EventID.ID
					if id != "" {
						if _, ok := seen[id]; !ok {
							seen[id] = struct{}{}
							eventIDs = append(eventIDs, id)
						}
					}
				}
			}
		}
	}

	util.LOGGER.Info("Event IDs found from schedules",
		"count", len(eventIDs),
		"event_ids", eventIDs,
	)

	// Nếu có filter thời gian nhưng không tìm thấy event nào → trả rỗng luôn
	if normalizedChoseDate != "" && len(eventIDs) == 0 {
		util.LOGGER.Info("No events found matching time filter", "chose_date", normalizedChoseDate)
		ctx.JSON(http.StatusOK, EventListResponse{
			Data: []Event{},
			Meta: &Meta{TotalCount: 0, FilterCount: 0, Limit: limit, Offset: offset},
		})
		return
	}

	// =========================================
	// Tạo filter cho bảng events
	// =========================================
	filters := BuildEventFilters(search, category, location, status)
	if len(eventIDs) > 0 {
		idFilter := fmt.Sprintf(`{"id":{"_in":["%s"]}}`, strings.Join(eventIDs, `","`))
		if filters == "" {
			filters = idFilter
		} else {
			filters = fmt.Sprintf(`{"_and":[%s,%s]}`, filters, idFilter)
		}
	}

	// =========================================
	// Gọi API Directus để lấy danh sách events
	// =========================================
	queryParams := url.Values{}
	if filters != "" {
		queryParams.Add("filter", filters)
	}

	queryParams.Add("fields", "*,category_id.id,category_id.name,image.id,preview_image.id")
	queryParams.Add("limit", strconv.Itoa(limit))
	queryParams.Add("offset", strconv.Itoa(offset))
	queryParams.Add("sort", sortField)
	queryParams.Add("meta", "total_count,filter_count")

	directusURL := fmt.Sprintf("%s/items/events?%s", server.config.DirectusAddr, queryParams.Encode())
	resp2, statusCode, err2 := util.MakeRequest("GET", directusURL, nil, server.config.DirectusStaticToken)
	if err2 != nil {
		util.LOGGER.Error("GET /api/events: failed to get events from Directus", "error", err2)
		ctx.JSON(statusCode, ErrorResponse{Message: err2.Error()})
		return
	}
	defer resp2.Body.Close()

	var directusResp struct {
		Data []map[string]interface{} `json:"data"`
		Meta *struct {
			TotalCount  int `json:"total_count"`
			FilterCount int `json:"filter_count"`
		} `json:"meta,omitempty"`
	}

	if err := json.NewDecoder(resp2.Body).Decode(&directusResp); err != nil {
		util.LOGGER.Error("GET /api/events: failed to decode Directus response", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Internal server error"})
		return
	}

	// =========================================
	// Map dữ liệu sang struct Event
	// =========================================
	events := make([]Event, 0, len(directusResp.Data))
	for _, item := range directusResp.Data {
		event := MapToEvent(item)
		if event.Image != "" {
			event.Image = util.CreateImageLink(event.Image)
		}
		events = append(events, event)
	}

	// Gắn lịch (start/end time) theo chose_date nếu có
	events = AttachScheduleToEvents(
		events,
		normalizedChoseDate,
		"", // ed (end-date filter) nếu sau này cần
		server.config.DirectusAddr,
		server.config.DirectusStaticToken,
	)

	response := EventListResponse{Data: events}
	if directusResp.Meta != nil {
		response.Meta = &Meta{
			TotalCount:  directusResp.Meta.TotalCount,
			FilterCount: directusResp.Meta.FilterCount,
			Limit:       limit,
			Offset:      offset,
		}
	}

	ctx.JSON(http.StatusOK, response)
}

// GET /api/events/:id
func (server *Server) GetEventByID(ctx *gin.Context) {
	id := ctx.Param("id")
	if id == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{Message: "Event ID is required"})
		return
	}

	queryParams := url.Values{}
	queryParams.Add("fields", "*,category_id.id,category_id.name,image.id,preview_image.id")

	directusURL := fmt.Sprintf("%s/items/events/%s?%s", server.config.DirectusAddr, id, queryParams.Encode())
	resp, statusCode, err := util.MakeRequest("GET", directusURL, nil, server.config.DirectusStaticToken)
	if err != nil {
		util.LOGGER.Error("GET /api/events/:id: failed to get event from Directus", "error", err, "id", id)
		ctx.JSON(statusCode, ErrorResponse{Message: err.Error()})
		return
	}
	defer resp.Body.Close()

	var directusResp struct {
		Data map[string]interface{} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
		util.LOGGER.Error("GET /api/events/:id: failed to decode Directus response", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{Message: "Internal server error"})
		return
	}

	event := MapToEvent(directusResp.Data)

	// Ở API detail, không có filter thời gian từ query → truyền rỗng để lấy lịch sớm nhất/có sẵn
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

	ctx.JSON(http.StatusOK, EventDetailResponse{Data: event})
}

// buildEventFilters tạo filter JSON cho bảng events từ các query cơ bản
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

// mapToEvent chuyển raw map từ Directus sang struct Event
func MapToEvent(data map[string]interface{}) Event {
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

	// ===== Category (giữ nguyên) =====
	if catObj, ok := data["category_id"].(map[string]interface{}); ok {
		if catName, ok := catObj["name"].(string); ok && catName != "" {
			event.Category = catName
		} else if catID, ok := catObj["id"].(string); ok {
			event.Category = catID
		}
	} else if catStr, ok := data["category_id"].(string); ok {
		event.Category = catStr
	}

	// ===== Image (mới): hỗ trợ cả string hoặc object, và fallback sang preview_image =====
	// 1) image
	if imgStr, ok := data["image"].(string); ok && imgStr != "" {
		event.Image = imgStr
	} else if imgObj, ok := data["image"].(map[string]interface{}); ok {
		if id, ok := imgObj["id"].(string); ok && id != "" {
			event.Image = id
		}
	}
	// 2) preview_image (fallback nếu image trống)
	if event.Image == "" {
		if pStr, ok := data["preview_image"].(string); ok && pStr != "" {
			event.Image = pStr
		} else if pObj, ok := data["preview_image"].(map[string]interface{}); ok {
			if id, ok := pObj["id"].(string); ok && id != "" {
				event.Image = id
			}
		}
	}

	if startTime, ok := data["start_time"].(string); ok {
		event.StartTime = startTime
	}
	if endTime, ok := data["end_time"].(string); ok {
		event.EndTime = endTime
	}

	// (Giữ nguyên các field còn lại)
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

// AttachScheduleToEvents gắn StartTime/EndTime từ event_schedules cho từng Event.
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
		// Logic: (start_time <= choseDate) AND (end_time >= choseDate) - event diễn ra trong ngày được chọn
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
	qp.Add("sort", "start_time") // để lấy lịch sớm nhất (hoặc sớm nhất trong khoảng)
	qp.Add("limit", "-1")        // nếu Directus cho phép unlimited

	u := fmt.Sprintf("%s/items/event_schedules?%s", directusAddr, qp.Encode())
	resp, _, err := util.MakeRequest("GET", u, nil, token)
	if err != nil || resp == nil {
		util.LOGGER.Error("AttachScheduleToEvents: failed to get schedules", "error", err)
		return events
	}
	defer resp.Body.Close()

	var payload struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		util.LOGGER.Error("AttachScheduleToEvents: decode error", "error", err)
		return events
	}

	// Gom theo event_id (Directus có thể trả event_id dạng string hoặc object)
	scheduleMap := make(map[string][]map[string]interface{})
	for _, s := range payload.Data {
		if id, ok := s["event_id"].(string); ok {
			scheduleMap[id] = append(scheduleMap[id], s)
			continue
		}
		if obj, ok := s["event_id"].(map[string]interface{}); ok {
			if id, ok := obj["id"].(string); ok {
				scheduleMap[id] = append(scheduleMap[id], s)
			}
		}
	}

	// Gắn lại start_time/end_time từ schedules (đã lọc nếu có choseDate)
	for i, e := range events {
		if list, ok := scheduleMap[e.ID]; ok && len(list) > 0 {
			// đã sort theo start_time nên list[0] là sớm nhất (trong khoảng nếu có filter)
			if st, ok := list[0]["start_time"].(string); ok {
				events[i].StartTime = st
			}
			if et, ok := list[0]["end_time"].(string); ok {
				events[i].EndTime = et
			}
		} else {
			// không có schedule khớp (đặc biệt khi có filter) → để trống để tránh hiểu lầm
			if choseDate != "" {
				events[i].StartTime = ""
				events[i].EndTime = ""
			}
		}
	}

	return events
}
