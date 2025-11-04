package api

import (
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
	EventName      string       `json:"name"`
	Address        string       `json:"address"`
	Avatar         string       `json:"preview_image"`
	EventSchedules []Start_time `json:"event_schedules"`
}

type Ticket_id struct {
	Description string `json:"description"`
}
type Payment struct {
	Amount string `json:"amount"`
}
type Booking_id struct {
	Payments []Payment `json:"payments"`
}
type Booking_items struct {
	Qr         string    `json:"qr"`
	Ticket_ids Ticket_id `json:"ticket_id"`
}
type MyOrder struct {
	ID          string    `json:"id"`
	Events      Event     `json:"event_id"`
	CreatedAt   time.Time `json:"date_created"`
	TicketCount string    `json:"ticket_count,omitempty"`
}

type Count_number struct {
	Count string `json:"count"`
}
type MyOrderResponse struct {
	Bookings []MyOrder `json:"bookings"`
	// TicketCounts []Count_number `json:"quantity_order"`
}

// Order_ Detail

type Order_detail struct {
	Events        Event           `json:"event_id"`
	CreatedAt     time.Time       `json:"date_created"`
	Booking_itemx []Booking_items `json:"booking_items"`
	Booking_idx   Booking_id      `json:"booking_id"`
}
type OrderDetailRespone struct {
	Order        []Order_detail `json:"order_detail"`
	Count_number []Count_number `json:"quantity_buy"`
}

func (server *Server) MyOrder(ctx *gin.Context) {
	layout := "2006-01-02"
	var time_use time.Time
	var time_next_use time.Time
	customer_id := ctx.Param("customer_id")
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

	booking_url := fmt.Sprintf(
		"%s/items/bookings?fields=id,date_created,event_id.name,event_id.address,event_id.preview_image,event_id.event_schedules.start_time&filter[customer_id][_eq]=%s&filter[date_created][_gte]=%s&filter[date_created][_lt]=%s",
		server.config.DirectusAddr,
		customer_id, time_use.Format(layout), time_next_use.Format(layout),
	)

	type bookings_result struct {
		Data   []MyOrder
		Status int
		Error  error
	}

	bookings_chan := make(chan bookings_result)

	// Goroutine: Fetch bookings
	go func() {
		var bookings []MyOrder

		status, err := util.MakeRequest("GET", booking_url, nil, server.config.DirectusStaticToken, &bookings)
		if err != nil {
			bookings_chan <- bookings_result{nil, status, err}
			return
		}
		bookings_chan <- bookings_result{bookings, status, nil}
	}()

	bookingsResult := <-bookings_chan

	// Kiểm tra lỗi
	if bookingsResult.Error != nil {
		util.LOGGER.Error("Failed to fetch bookings", "error", bookingsResult.Error)
		ctx.JSON(bookingsResult.Status, ErrorResponse{bookingsResult.Error.Error()})
		return
	}

	if len(bookingsResult.Data) == 0 {
		ctx.JSON(http.StatusOK, MyOrderResponse{Bookings: []MyOrder{}})
		return
	}

	// Lấy ticket count cho từng booking
	type countResult struct {
		index int
		count string
		err   error
	}

	countChan := make(chan countResult, len(bookingsResult.Data))

	// Tạo goroutine cho mỗi booking
	for i, booking := range bookingsResult.Data {
		go func(index int, bookingID string) {
			number_booking_ticket := fmt.Sprintf(
				"%s/items/booking_items?filter[booking_id][_eq]=%s&aggregate[count]=*",
				server.config.DirectusAddr,
				bookingID,
			)

			var ticketCounts []Count_number

			_, err := util.MakeRequest("GET", number_booking_ticket, nil, server.config.DirectusStaticToken, &ticketCounts)

			ticketCount := "0"
			if err == nil && len(ticketCounts) > 0 {
				ticketCount = ticketCounts[0].Count
			}

			countChan <- countResult{
				index: index,
				count: ticketCount,
				err:   err,
			}
		}(i, booking.ID)
	}

	// Thu thập kết quả
	bookingsWithCount := make([]MyOrder, len(bookingsResult.Data))
	for i := 0; i < len(bookingsResult.Data); i++ {
		res := <-countChan
		bookingsWithCount[res.index] = MyOrder{
			ID:          bookingsResult.Data[res.index].ID,
			Events:      bookingsResult.Data[res.index].Events,
			CreatedAt:   bookingsResult.Data[res.index].CreatedAt,
			TicketCount: res.count,
		}
		if res.err != nil {
			util.LOGGER.Warn("Failed to fetch ticket count", "index", res.index, "error", res.err)
		}
	}

	response := MyOrderResponse{
		Bookings: bookingsWithCount,
	}

	ctx.JSON(http.StatusOK, response)
}

//

func (server *Server) OrderDetail(ctx *gin.Context) {
	booking_id := ctx.Param("booking_id")
	booking_url := fmt.Sprintf(
		"%s/items/bookings?fields=date_created,event_id.name,event_id.address,event_id.preview_image,event_id.event_schedules.start_time,booking_items.qr,booking_items.ticket_id.description,booking_items.booking_id.payments.amount&filter[id][_eq]=%s",
		server.config.DirectusAddr,
		booking_id,
	)

	number_booking_ticket := fmt.Sprintf("%s/items/booking_items?filter[booking_id][_eq]=%s&aggregate[count]=*", server.config.DirectusAddr, booking_id)

	type Order_detail_result struct {
		Data   []Order_detail
		Status int
		Error  error
	}

	type number_ticket_result struct {
		Data   []Count_number
		Status int
		Error  error
	}

	order_detail_chan := make(chan Order_detail_result)
	number_ticket_result_chan := make(chan number_ticket_result)

	// Goroutine 1: Fetch bookings
	go func() {
		var order []Order_detail

		status, err := util.MakeRequest("GET", booking_url, nil, server.config.DirectusStaticToken, &order)
		if err != nil {
			order_detail_chan <- Order_detail_result{nil, status, err}
			return
		}
		order_detail_chan <- Order_detail_result{order, status, nil}
	}()

	// Goroutine 2: Fetch ticket count
	go func() {
		var ticketCounts []Count_number

		status, err := util.MakeRequest("GET", number_booking_ticket, nil, server.config.DirectusStaticToken, &ticketCounts)
		if err != nil {
			number_ticket_result_chan <- number_ticket_result{nil, status, err}
			return
		}
		number_ticket_result_chan <- number_ticket_result{ticketCounts, status, nil}
	}()

	orderDetailResult := <-order_detail_chan
	ticketCountResult := <-number_ticket_result_chan

	// Kiểm tra lỗi
	if orderDetailResult.Error != nil {
		util.LOGGER.Error("Failed to fetch bookings", "error", orderDetailResult.Error)
		ctx.JSON(orderDetailResult.Status, ErrorResponse{orderDetailResult.Error.Error()})
		return
	}

	if ticketCountResult.Error != nil {
		util.LOGGER.Error("Failed to fetch ticket count", "error", ticketCountResult.Error)
		ctx.JSON(ticketCountResult.Status, ErrorResponse{ticketCountResult.Error.Error()})
		return
	}
	response := OrderDetailRespone{
		Order:        orderDetailResult.Data,
		Count_number: ticketCountResult.Data,
	}

	ctx.JSON(http.StatusOK, response)
}

// My ticket

func (server *Server) MyTicket(ctx *gin.Context) {
	layout := "2006-01-02"
	var time_use time.Time
	var time_next_use time.Time
	booking_id := ctx.Param("booking_id")
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

	booking_url := fmt.Sprintf(
		"%s/items/booking_items?fields=date_created,booking_id.event_id.name,booking_id.event_id.address,booking_id.event_id.preview_image,booking_id.event_id.event_schedules.start_time&filter[booking_id][_eq]=%s&filter[date_created][_gte]=%s&filter[date_created][_lt]=%s",
		server.config.DirectusAddr, booking_id,
		time_use.Format(layout), time_next_use.Format(layout),
	)

	type bookings_result struct {
		Data   []MyOrder
		Status int
		Error  error
	}

	bookings_chan := make(chan bookings_result)

	// Goroutine: Fetch bookings
	go func() {
		var bookings []MyOrder

		status, err := util.MakeRequest("GET", booking_url, nil, server.config.DirectusStaticToken, &bookings)
		if err != nil {
			bookings_chan <- bookings_result{nil, status, err}
			return
		}
		bookings_chan <- bookings_result{bookings, status, nil}
	}()

	bookingsResult := <-bookings_chan

	// Kiểm tra lỗi
	if bookingsResult.Error != nil {
		util.LOGGER.Error("Failed to fetch bookings", "error", bookingsResult.Error)
		ctx.JSON(bookingsResult.Status, ErrorResponse{bookingsResult.Error.Error()})
		return
	}

	if len(bookingsResult.Data) == 0 {
		ctx.JSON(http.StatusOK, MyOrderResponse{Bookings: []MyOrder{}})
		return
	}

	// Lấy ticket count cho từng booking
	type countResult struct {
		index int
		count string
		err   error
	}

	countChan := make(chan countResult, len(bookingsResult.Data))

	// Tạo goroutine cho mỗi booking
	for i, booking := range bookingsResult.Data {
		go func(index int, bookingID string) {
			number_booking_ticket := fmt.Sprintf(
				"%s/items/booking_items?filter[booking_id][_eq]=%s&aggregate[count]=*",
				server.config.DirectusAddr,
				bookingID,
			)

			var ticketCounts []Count_number

			_, err := util.MakeRequest("GET", number_booking_ticket, nil, server.config.DirectusStaticToken, &ticketCounts)

			ticketCount := "0"
			if err == nil && len(ticketCounts) > 0 {
				ticketCount = ticketCounts[0].Count
			}

			countChan <- countResult{
				index: index,
				count: ticketCount,
				err:   err,
			}
		}(i, booking.ID)
	}

	// Thu thập kết quả
	bookingsWithCount := make([]MyOrder, len(bookingsResult.Data))
	for i := 0; i < len(bookingsResult.Data); i++ {
		res := <-countChan
		bookingsWithCount[res.index] = MyOrder{
			ID:          bookingsResult.Data[res.index].ID,
			Events:      bookingsResult.Data[res.index].Events,
			CreatedAt:   bookingsResult.Data[res.index].CreatedAt,
			TicketCount: res.count,
		}
		if res.err != nil {
			util.LOGGER.Warn("Failed to fetch ticket count", "index", res.index, "error", res.err)
		}
	}

	response := MyOrderResponse{
		Bookings: bookingsWithCount,
	}

	ctx.JSON(http.StatusOK, response)
}
