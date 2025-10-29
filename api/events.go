package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"tekticket/util"

	"github.com/gin-gonic/gin"
)

type EventResponse struct {
	ID           string `json:"id"`
	CreatorID    string `json:"creator_id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	CategoryID   string `json:"category_id"`
	Address      string `json:"address"`
	City         string `json:"city"`
	Country      string `json:"country"`
	PreviewImage string `json:"preview_image"`
	Slug         string `json:"slug"`
	Status       string `json:"status"`
}

type TicketResponse struct {
	ID          string  `json:"id"`
	EventID     string  `json:"event_id"`
	SeatZoneID  string  `json:"seat_zone_id"`
	Rank        string  `json:"rank"`
	Description string  `json:"description"`
	BasePrice   float64 `json:"base_price"`
	Status      string  `json:"status"`
}

// GetEventTickets godoc
// @Summary      Get all tickets for an event
// @Description  Retrieves all available ticket types for a specific event
// @Tags         Events
// @Accept       json
// @Produce      json
// @Param        event_id  path      string  true  "Event ID"
// @Success      200  {array}   TicketResponse  "List of tickets"
// @Failure      400  {object}  ErrorResponse   "Invalid event ID"
// @Failure      500  {object}  ErrorResponse   "Internal server error"
// @Router       /api/events/tickets/:event_id [get]
func (server *Server) GetEventTickets(ctx *gin.Context) {
	eventID := ctx.Param("event_id")
	if eventID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Event ID is required"})
		return
	}

	// Get tickets from Directus
	url := fmt.Sprintf("%s/items/tickets?filter[event_id][_eq]=%s&fields=*",
		server.config.DirectusAddr, eventID)

	resp, status, err := util.MakeRequest("GET", url, nil, server.config.DirectusStaticToken)
	if err != nil {
		util.LOGGER.Error("GET /api/events/tickets/:event_id: failed to fetch tickets", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	var directusResp struct {
		Data []TicketResponse `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
		util.LOGGER.Error("GET /api/events/:event_id/tickets: failed to decode response", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"data": directusResp.Data})
}
