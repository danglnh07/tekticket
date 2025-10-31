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

// Response structure
type MembershipResponse struct {
	Points       int     `json:"points"`
	Tier         string  `json:"tier"`
	Discount     float64 `json:"discount"`
	EarlyBuyTime int     `json:"early_buy_time"`
}

// Implement float64 custom unmarshal JSON that handle both float and string
type DecimalFloat float64

func (df *DecimalFloat) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as float64 first
	var f float64
	if err := json.Unmarshal(data, &f); err == nil {
		*df = DecimalFloat(f)
		return nil
	}

	// If that fails, try as string
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	// Parse string to float
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}

	*df = DecimalFloat(f)
	return nil
}

type MembershipTier struct {
	ID           string       `json:"id"`
	Tier         string       `json:"tier"`
	BasePoint    int          `json:"base_point"`
	Discount     DecimalFloat `json:"discount"` // Can be string or number from Directus
	EarlyBuyTime int          `json:"early_buy_time"`
	Status       string       `json:"status"`
}

// User memberships log. Since we only use this to calculate the current total membership points, we don't return
// (and should never return) to the end user
type UserMembershipLog struct {
	ID             string `json:"id"`
	ResultingPoint int    `json:"resulting_points"`
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
// @Security BearerAuth
// @Router       /api/memberships/{user_id} [get]
func (server *Server) GetUserMembership(ctx *gin.Context) {
	// Get access token
	token := server.GetToken(ctx)
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Unauthorized access"})
		return
	}

	// Get user ID
	userID := ctx.Param("id")
	if userID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"User ID is required"})
		return
	}

	// To get the user current point, we just need to get the latest log of that user, resulting_points would be the current point
	queryParams := url.Values{}
	fields := []string{"id", "resulting_points"}
	queryParams.Add("fields", strings.Join(fields, ","))
	queryParams.Add("filter[customer_id][_eq]", userID)
	queryParams.Add("sort", "-date_updated")
	directusURL := fmt.Sprintf("%s/items/user_membership_logs/?%s", server.config.DirectusAddr, queryParams.Encode())

	// Make request to Directus
	resp, status, err := util.MakeRequest("GET", directusURL, nil, token)
	if err != nil {
		util.LOGGER.Error("GET api/memberships/:id: failed to make request to Directus", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}
	defer resp.Body.Close()

	// Parse Directus response
	var directusResp struct {
		Data []UserMembershipLog `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
		util.LOGGER.Error("GET /api/memberships/:id: failed to parse Directus response", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// When an account is created, their would be a log created for them, so we can assume that at least 1 record is returned
	result := MembershipResponse{
		Points: directusResp.Data[0].ResultingPoint,
	}

	// Get the list of all membership to determine the current user rank and privilege
	// Since the membership return should be sorted by its base point, we just have to iterate over it and find the largest
	// tier with base point lower or equal than current point
	memberships := server.listMemberships(ctx)
	if memberships == nil {
		return
	}

	for _, membership := range memberships {
		if membership.BasePoint <= result.Points {
			result.Tier = membership.Tier
			result.EarlyBuyTime = membership.EarlyBuyTime
			result.Discount = float64(membership.Discount)
		} else {
			break
		}
	}

	ctx.JSON(http.StatusOK, result)
}

// ListMemberships godoc
// @Summary      List all published membership tiers
// @Description  Retrieves all published membership tiers sorted by resulting points in ascending order.
// @Tags         Memberships
// @Accept       json
// @Produce      json
// @Success      200  {array}   MembershipTier    "List of memberships retrieved successfully"
// @Failure      401  {object}  ErrorResponse     "Unauthorized access"
// @Failure      500  {object}  ErrorResponse     "Internal server error or failed to communicate with Directus"
// @Security BearerAuth
// @Router       /api/memberships [get]
func (server *Server) ListMemberships(ctx *gin.Context) {
	// Get the list of all memberships
	memberships := server.listMemberships(ctx)
	if memberships == nil {
		// The helper already handle error return to client, so we just return here
		return
	}
	ctx.JSON(http.StatusOK, memberships)
}

// Helper method: Get the list of all memberships
func (server *Server) listMemberships(ctx *gin.Context) []MembershipTier {
	// Get access token
	token := server.GetToken(ctx)
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Unauthorized access"})
		return nil
	}

	// Get the list of all memberships. It should be a short list, so we don't need to provide any paging here
	url := fmt.Sprintf("%s/items/memberships?filter[status][_eq]=published&sort=base_point", server.config.DirectusAddr)
	resp, status, err := util.MakeRequest("GET", url, nil, token)
	if err != nil {
		util.LOGGER.Error(
			fmt.Sprintf("%s %s: failed to get the list of all memberships", ctx.Request.Method, ctx.FullPath()), "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return nil
	}
	defer resp.Body.Close()

	// Parse Directus response
	var directusResp struct {
		Data []MembershipTier `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
		util.LOGGER.Error(
			fmt.Sprintf("%s %s: failed to get the list of all memberships", ctx.Request.Method, ctx.FullPath()), "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return nil
	}

	return directusResp.Data
}
