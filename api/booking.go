package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"tekticket/util"
	"time"

	"github.com/gin-gonic/gin"
)

type Start_time struct {
	Start_time string `json:"start_time"`
}

type Event struct {
	EvenName       string       `json:"name"`
	Address        string       `json:"address"`
	Avatar         string       `json:"preview_image"`
	EventSchedules []Start_time `json:"event_schedules"`
}
type MyOrder struct {
	CreatedAt time.Time `json:"date_created"`
	Events    Event     `json:"event_id"`
}
type bookings_res struct {
	Data []MyOrder `json:"data"`
}

type Count_number struct {
	Count int `json:"count"`
}
type number_ticket_res struct {
	Data []Count_number `json:"data"`
}

// Get MyBooking
func (server *Server) GetMyOrder(ctx *gin.Context) {
	layout := "2006-01-02"
	var time_use time.Time
	var time_next_use time.Time
	customer_id := ctx.Param("id")
	booking_id := ctx.Param("Booking_id")
	date := ctx.Param("date")
	var err error
	if date == "" {
		time_use = time.Now()
		time_next_use = time_use.AddDate(0, 0, 1)
	} else {
		time_use, err = time.Parse(layout, date)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid date format"})
			return
		}
		time_next_use = time_use.AddDate(0, 0, 1)
	}

	// http://localhost:8055/items/bookings?fields=date_created, event_id.name,event_id.address,event_id.preview_image,event_id.event_schedules.start_time&filter=[customer_id][_eq]=f7cce5a6-bcca-41e5-a2c8-f56042c3b4c3

	booking_url := fmt.Sprintf(
		"%s/items/bookings?fields=date_created,event_id.name,event_id.address,event_id.preview_image,event_id.event_schedules.start_time&filter[customer_id][_eq]=%s&filter[date_created][_gte]=%s&filter[date_created][_lt]=%s",
		server.config.DirectusAddr,
		customer_id, time_use.Format(layout), time_next_use.Format(layout),
	)
	number_booking_ticket := fmt.Sprintf("%s/items/booking_items?filter[booking_id][_eq]=%s&aggregate[count]=*", server.config.DirectusAddr, booking_id)

	type bookings_result struct {
		Data   []MyOrder
		Status int
		Error  error
	}

	type number_ticket_result struct {
		Data   Count_number
		Status int
		Error  error
	}
	bookings_chan := make(chan bookings_result)
	number_ticket_result_chan := make(chan number_ticket_result)
	go func() {
		resp, status, err := util.MakeRequest("GET", booking_url, nil, server.config.DirectusStaticToken)
		if err != nil {
			bookings_chan <- bookings_result{nil, status, err}
			return
		}
		defer resp.Body.Close()

		var directusResp struct {
			Data []MyOrder `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
			bookings_chan <- bookings_result{nil, http.StatusInternalServerError, err}
			return
		}
		bookings_chan <- bookings_result{directusResp.Data, status, nil}
	}()

	go func() {
		resp, status, err := util.MakeRequest("GET", number_booking_ticket, nil, server.config.DirectusStaticToken)
		if err != nil {

			number_ticket_result_chan <- number_ticket_result{nil, status, err}
			return
		}
		defer resp.Body.Close()
		var directusResp struct {
			Data []Count_number `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
			number_ticket_result_chan <- number_ticket_result{nil, http.StatusInternalServerError, err}
			return
		}
		number_ticket_result_chan <- number_ticket_result{directusResp.Data, status, nil}
	}()

	// Đợi cả 2 kết quả
	bookings_res := <-bookings_chan
	number_ticket_res := <-number_ticket_result_chan

	// Kiểm tra lỗi
	if bookings_res.Error != nil {
		util.LOGGER.Error("Failed to fetch bookings", "error", bookings_res.Error)
		ctx.JSON(bookings_res.Status, ErrorResponse{bookings_res.Error.Error()})
		return
	}

	if number_ticket_res.Error != nil {
		util.LOGGER.Error("Failed to fetch customer", "error", number_ticket_res.Error)
		ctx.JSON(number_ticket_res.Status, ErrorResponse{number_ticket_res.Error.Error()})
		return
	}

	response := gin.H{
		"bookings":      bookings_res.Data,
		"ticket_counts": number_ticket_res.Data,
		"meta": gin.H{
			"date":           date,
			"bookings_count": len(bookings_res.Data),
		},
	}

	ctx.JSON(http.StatusOK, response)

}
