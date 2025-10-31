package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"tekticket/util"

	"github.com/gin-gonic/gin"
)

// Response structure
type MembershipResponse struct {
	UserID        string  `json:"user_id"`
	Points        int     `json:"points"`
	Tier          string  `json:"tier"`
	Discount      float64 `json:"discount"`
	EarlyBuyTime  int     `json:"early_buy_time"`
	LifetimeSpent float64 `json:"Amount"`
}

type MembershipTier struct {
	ID           string      `json:"id"`
	Tier         string      `json:"tier"`
	BasePoint    int         `json:"base_point"`
	Discount     interface{} `json:"discount"` // Can be string or number from Directus
	EarlyBuyTime int         `json:"early_buy_time"`
	Status       string      `json:"status"`
}

// GetUserMembership godoc
// @Summary      Get customer membership info
// @Description  Calculate points from total payments (100,000 VND = 10 points) and determine tier
// @Tags         Memberships
// @Accept       json
// @Produce      json
// @Param        user_id  path      string  true  "User ID"
// @Success      200  {object}  MembershipResponse  "Customer membership info"
// @Failure      400  {object}  ErrorResponse        "Invalid user ID"
// @Failure      500  {object}  ErrorResponse        "Internal server error"
// @Router       /api/memberships/{user_id} [get]
func (server *Server) GetUserMembership(ctx *gin.Context) {
	userID := ctx.Param("user_id")
	if userID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"User ID is required"})
		return
	}

	// Step 1: Get all bookings for this customer
	bookingsURL := fmt.Sprintf("%s/items/bookings?filter[customer_id][_eq]=%s&fields=id",
		server.config.DirectusAddr, userID)
	bookingsResp, status, err := util.MakeRequest("GET", bookingsURL, nil, server.config.DirectusStaticToken)
	if err != nil {
		util.LOGGER.Error("GET /api/memberships/:user_id: failed to get bookings", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	var bookingsData struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(bookingsResp.Body).Decode(&bookingsData); err != nil {
		util.LOGGER.Error("GET /api/memberships/:user_id: failed to decode bookings", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Step 2: Calculate total payments for these bookings
	lifetimeSpent := 0.0
	if len(bookingsData.Data) > 0 {
		// Get sum of successful payments for each booking
		for _, bookingID := range bookingsData.Data {
			paymentURL := fmt.Sprintf("%s/items/payments?aggregate[sum]=amount&filter[status][_eq]=success&filter[booking_id][_eq]=%s",
				server.config.DirectusAddr, bookingID.ID)
			paymentResp, _, err := util.MakeRequest("GET", paymentURL, nil, server.config.DirectusStaticToken)
			if err == nil {
				var paymentData struct {
					Data []struct {
						Sum struct {
							Amount string `json:"amount"` // Directus returns as string
						} `json:"sum"`
					} `json:"data"`
				}
				if err := json.NewDecoder(paymentResp.Body).Decode(&paymentData); err == nil {
					if len(paymentData.Data) > 0 {
						if amount, err := strconv.ParseFloat(paymentData.Data[0].Sum.Amount, 64); err == nil {
							lifetimeSpent += amount
						}
					}
				}
			}
		}
	}

	// Step 3: Calculate points (100,000 VND = 10 points)
	points := int(lifetimeSpent / 10000) // 100000 VND = 10 points, so divide by 10000

	// Step 3: Get all published membership tiers from Directus
	tiersURL := fmt.Sprintf("%s/items/memberships?filter[status][_eq]=published&sort[]=base_point&fields=*",
		server.config.DirectusAddr)
	tiersResp, status, err := util.MakeRequest("GET", tiersURL, nil, server.config.DirectusStaticToken)
	if err != nil {
		util.LOGGER.Error("GET /api/memberships/:user_id: failed to get tiers", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	var tiersData struct {
		Data []MembershipTier `json:"data"`
	}

	if err := json.NewDecoder(tiersResp.Body).Decode(&tiersData); err != nil {
		util.LOGGER.Error("GET /api/memberships/:user_id: failed to decode tiers", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Step 4: Determine tier based on points
	tier, discount, earlyBuyTime := determineTierByPoints(tiersData.Data, points)

	// Step 5: Return response
	ctx.JSON(http.StatusOK, MembershipResponse{
		UserID:        userID,
		Points:        points,
		Tier:          tier,
		Discount:      discount,
		EarlyBuyTime:  earlyBuyTime,
		LifetimeSpent: lifetimeSpent,
	})
}

// Helper function to safely parse discount from interface{} (can be string, number, or null)
func parseDiscount(val interface{}) float64 {
	if val == nil {
		return 0.0
	}

	switch v := val.(type) {
	case float64:
		return v
	case string:
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			return parsed
		}
	case int:
		return float64(v)
	}

	return 0.0
}

// Helper function to determine tier based on current points
func determineTierByPoints(tiers []MembershipTier, points int) (string, float64, int) {
	// Default values
	selectedTier := "bronze"
	discount := 0.0
	earlyBuyTime := 0

	// Find highest tier that customer qualifies for based on points
	for _, tier := range tiers {
		if points >= tier.BasePoint {
			selectedTier = tier.Tier
			discount = parseDiscount(tier.Discount)
			earlyBuyTime = tier.EarlyBuyTime
		}
	}

	return selectedTier, discount, earlyBuyTime
}
