package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"tekticket/util"

	"github.com/gin-gonic/gin"
)

type SeatResponse struct {
	ID         string  `json:"id"`
	SeatZoneID string  `json:"seat_zone_id"`
	SeatNumber string  `json:"seat_number"`
	Status     string  `json:"status"` // empty, reserved, booked
	ReservedBy *string `json:"reserved_by,omitempty"`
}

type SeatZoneResponse struct {
	ID          string `json:"id"`
	EventID     string `json:"event_id"`
	Description string `json:"description"`
	TotalSeats  int    `json:"total_seats"`
	Status      string `json:"status"`
}

type EventInfoResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	City      string `json:"city"`
	Country   string `json:"country"`
	StartDate string `json:"start_date,omitempty"`
	EndDate   string `json:"end_date,omitempty"`
}

type GetSeatsResponse struct {
	Event    EventInfoResponse `json:"event"`
	SeatZone SeatZoneResponse  `json:"seat_zone"`
	Seats    []SeatResponse    `json:"seats"`
}

// GetSeats godoc
// @Summary      Get all seats in a seat zone
// @Description  Retrieves all seats and their status for a specific seat zone
// @Tags         Seats
// @Accept       json
// @Produce      json
// @Param        seat_zone_id  path      string  true  "Seat Zone ID"
// @Success      200  {object}  GetSeatsResponse  "Seat zone and seats information"
// @Failure      400  {object}  ErrorResponse     "Invalid seat zone ID"
// @Failure      404  {object}  ErrorResponse     "Seat zone not found"
// @Failure      500  {object}  ErrorResponse     "Internal server error"
// @Router       /api/seat-zones/{seat_zone_id} [get]
func (server *Server) GetSeats(ctx *gin.Context) {
	seatZoneID := ctx.Param("seat_zone_id")
	if seatZoneID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Seat zone ID is required"})
		return
	}

	// Get seat zone information
	zoneURL := fmt.Sprintf("%s/items/seat_zones/%s", server.config.DirectusAddr, seatZoneID)
	zoneResp, status, err := util.MakeRequest("GET", zoneURL, nil, server.config.DirectusStaticToken)
	if err != nil {
		util.LOGGER.Error("GET /api/seat-zones/:seat_zone_id/seats: failed to fetch seat zone", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	var zoneData struct {
		Data SeatZoneResponse `json:"data"`
	}

	if err := json.NewDecoder(zoneResp.Body).Decode(&zoneData); err != nil {
		util.LOGGER.Error("GET /api/seat-zones/:seat_zone_id/seats: failed to decode zone response", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Get event information
	eventURL := fmt.Sprintf("%s/items/events/%s?fields=id,name,address,city,country",
		server.config.DirectusAddr, zoneData.Data.EventID)

	eventResp, status, err := util.MakeRequest("GET", eventURL, nil, server.config.DirectusStaticToken)
	if err != nil {
		util.LOGGER.Error("GET /api/seat-zones/:seat_zone_id/seats: failed to fetch event", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	var eventData struct {
		Data EventInfoResponse `json:"data"`
	}

	if err := json.NewDecoder(eventResp.Body).Decode(&eventData); err != nil {
		util.LOGGER.Error("GET /api/seat-zones/:seat_zone_id/seats: failed to decode event response", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Get event schedule information for start_time and end_time
	scheduleURL := fmt.Sprintf("%s/items/event_schedules?filter[event_id][_eq]=%s&limit=1&fields=start_time,end_time",
		server.config.DirectusAddr, zoneData.Data.EventID)

	scheduleResp, _, err := util.MakeRequest("GET", scheduleURL, nil, server.config.DirectusStaticToken)
	if err == nil {
		var scheduleData struct {
			Data []struct {
				StartTime string `json:"start_time"`
				EndTime   string `json:"end_time"`
			} `json:"data"`
		}

		if err := json.NewDecoder(scheduleResp.Body).Decode(&scheduleData); err == nil && len(scheduleData.Data) > 0 {
			eventData.Data.StartDate = scheduleData.Data[0].StartTime
			eventData.Data.EndDate = scheduleData.Data[0].EndTime
		}
	}

	// Get all seats in this zone
	seatsURL := fmt.Sprintf("%s/items/seats?filter[seat_zone_id][_eq]=%s&fields=*",
		server.config.DirectusAddr, seatZoneID)

	seatsResp, status, err := util.MakeRequest("GET", seatsURL, nil, server.config.DirectusStaticToken)
	if err != nil {
		util.LOGGER.Error("GET /api/seat-zones/:seat_zone_id/seats: failed to fetch seats", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	var seatsData struct {
		Data []SeatResponse `json:"data"`
	}

	if err := json.NewDecoder(seatsResp.Body).Decode(&seatsData); err != nil {
		util.LOGGER.Error("GET /api/seat-zones/:seat_zone_id/seats: failed to decode seats response", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"data": GetSeatsResponse{
			Event:    eventData.Data,
			SeatZone: zoneData.Data,
			Seats:    seatsData.Data,
		},
	})
}
