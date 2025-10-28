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
	Data []util.Event `json:"data"`
	Meta *Meta        `json:"meta,omitempty"`
}

// EventDetailResponse represents the response for event detail
type EventDetailResponse struct {
	Data util.Event `json:"data"`
}

// Meta represents pagination metadata
type Meta struct {
	TotalCount  int `json:"total_count"`
	FilterCount int `json:"filter_count"`
	Limit       int `json:"limit"`
	Offset      int `json:"offset"`
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
		// Chuyển thành ngày bắt đầu và kết thúc của ngày đó
		return d + "T00:00:00Z"
	}
	normalizedChoseDate := normalizeChoseDate(choseDate)

	// Debug log để kiểm tra chuẩn hóa ngày
	util.LOGGER.Info("Date normalization", "original_chose_date", choseDate, "normalized_chose_date", normalizedChoseDate)

	// =========================================
	// Lấy danh sách event_id từ bảng event_schedules nếu có filter thời gian
	// =========================================
	var eventIDs []string
	util.LOGGER.Info("Time filter check", "chose_date", normalizedChoseDate, "has_time_filter", normalizedChoseDate != "")
	if normalizedChoseDate != "" {
		// Lọc events có chose_date nằm trong khoảng thời gian của event
		// Logic: (start_time <= chose_date) AND (end_time >= chose_date) - event diễn ra trong ngày được chọn
		filterStr := fmt.Sprintf(`{"_and":[{"start_time":{"_lte":"%s"}},{"end_time":{"_gte":"%s"}}]}`, normalizedChoseDate, normalizedChoseDate)

		// Debug log để kiểm tra filter string
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

	// Debug log để kiểm tra số lượng eventIDs tìm thấy
	util.LOGGER.Info("Event IDs found from schedules", "count", len(eventIDs), "event_ids", eventIDs)

	// Nếu user có filter thời gian nhưng KHÔNG tìm thấy event nào khớp schedules → trả rỗng luôn
	if normalizedChoseDate != "" && len(eventIDs) == 0 {
		util.LOGGER.Info("No events found matching time filter", "chose_date", normalizedChoseDate)
		ctx.JSON(http.StatusOK, EventListResponse{
			Data: []util.Event{},
			Meta: &Meta{TotalCount: 0, FilterCount: 0, Limit: limit, Offset: offset},
		})
		return
	}

	// =========================================
	// Tạo filter cho bảng events
	// =========================================
	filters := util.BuildEventFilters(search, category, location, status)
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
	queryParams.Add("fields", "*,category_id")
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
	events := make([]util.Event, 0, len(directusResp.Data))
	for _, item := range directusResp.Data {
		event := util.MapToEvent(item)
		if event.Image != "" {
			event.Image = util.CreateImageLink(event.Image)
		}
		events = append(events, event)
	}

	events = util.AttachScheduleToEvents(events, normalizedChoseDate, "", server.config.DirectusAddr, server.config.DirectusStaticToken)

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
	queryParams.Add("fields", "*,category_id")

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

	event := util.MapToEvent(directusResp.Data)
	// Ở API detail, không có filter thời gian từ query → truyền rỗng để lấy lịch sớm nhất/có sẵn
	events := util.AttachScheduleToEvents([]util.Event{event}, "", "", server.config.DirectusAddr, server.config.DirectusStaticToken)
	if len(events) > 0 {
		event = events[0]
	}
	if event.Image != "" {
		event.Image = util.CreateImageLink(event.Image)
	}

	ctx.JSON(http.StatusOK, EventDetailResponse{Data: event})
}
