package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gosimple/slug"
	"github.com/skip2/go-qrcode"
)

// Global logger
var LOGGER = slog.New(slog.NewTextHandler(os.Stdout, nil))

const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Generate a random string with length n. The character possible is defined in the alphabet constant
func RandomString(n int) string {
	var sb strings.Builder
	k := len(alphabet)

	for range n {
		c := alphabet[rand.Intn(k)]
		sb.WriteByte(c)
	}

	return sb.String()
}

// Generate QR
func GenerateQR(content string) ([]byte, error) {
	return qrcode.Encode(content, qrcode.Medium, 256)
}

// Generate slug
func GenerateSlug(content string) string {
	return slug.Make(content)
}

// Generate random OTP code (6 digits code)
func GenerateRandomOTP() string {
	return fmt.Sprintf("%d", rand.Intn(999999-100000+1)+100000)
}

// Helper: make request to Directus
func MakeRequest(method, url string, body map[string]any, token string) (*http.Response, int, error) {
	var (
		req *http.Request
		err error
	)

	if body != nil {
		// build body
		data, err := json.Marshal(body)
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
		req, err = http.NewRequest(method, url, bytes.NewBuffer(data))
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
	} else {
		req, err = http.NewRequest(method, url, nil)
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
	}

	// Set request header
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	// Make request to Directus API
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	// Check if status code is success
	if 200 > resp.StatusCode || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(resp.Body)
		return nil, resp.StatusCode, fmt.Errorf("response status not ok: %s", string(message)+" "+resp.Status)
	}

	return resp, resp.StatusCode, nil
}

// Generate the URL of image using its ID
func CreateImageLink(id string) string {
	return fmt.Sprintf("http://localhost:8080/images/%s", id)
}

// =======================
// Event Helpers
// =======================

// Event represents an event object
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

	if catStr, ok := data["category_id"].(string); ok {
		event.Category = catStr
	}

	if startTime, ok := data["start_time"].(string); ok {
		event.StartTime = startTime
	}
	if endTime, ok := data["end_time"].(string); ok {
		event.EndTime = endTime
	}
	if image, ok := data["image"].(string); ok {
		event.Image = image
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
	resp, _, err := MakeRequest("GET", u, nil, token)
	if err != nil || resp == nil {
		LOGGER.Error("AttachScheduleToEvents: failed to get schedules", "error", err)
		return events
	}
	defer resp.Body.Close()

	var payload struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		LOGGER.Error("AttachScheduleToEvents: decode error", "error", err)
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
