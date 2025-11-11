package api

import (
	"fmt"
	"net/http"
	"net/url"
	"tekticket/db"
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

// GetUserMembership godoc
// @Summary      Get customer membership info
// @Description  Calculate points from total payments (100,000 VND = 10 points) and determine tier
// @Tags         Memberships
// @Accept       json
// @Produce      json
// @Success      200  {object}  MembershipResponse  "Customer membership info"
// @Failure      400  {object}  ErrorResponse        "Invalid user ID"
// @Failure      500  {object}  ErrorResponse        "Internal server error"
// @Security BearerAuth
// @Router       /api/memberships/me [get]
func (server *Server) GetUserMembership(ctx *gin.Context) {
	// Get access token
	token := server.GetToken(ctx)

	// Get user ID
	userID, err := util.ExtractIDFromToken(token)
	if err != nil {
		util.LOGGER.Error("GET /api/memberships/me: failed to get user ID from access token", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid token"})
		return
	}

	// To get the user current point, we just need to get the latest log of that user, resulting_points would be the current point
	// There are 2 ways to obtain this, through users.user_membership_logs or user_membership_logs with customer_id = userID
	queryParams := url.Values{}
	queryParams.Add("filter[customer_id][_eq]", userID)
	queryParams.Add("sort", "-date_updated")
	directusURL := fmt.Sprintf("%s/items/user_membership_logs/?%s", server.config.DirectusAddr, queryParams.Encode())

	// Make request to Directus
	var logs []db.UserMembershipLog
	status, err := db.MakeRequest("GET", directusURL, nil, token, &logs)
	if err != nil {
		util.LOGGER.Error("GET api/memberships/me: failed to make request to Directus", "status", status, "error", err)
		server.DirectusError(ctx, err)
		return
	}

	result := MembershipResponse{}
	if len(logs) != 0 {
		result.Points = logs[0].ResultingPoints
	} else {
		// If no log returned, we just assume this to be a new account -> point = 0 (although Directus would ensure this not to happen)
		result.Points = 0
	}

	// Get the list of all membership to determine the current user rank and privilege
	// Since the membership return should be sorted by its base point, we just have to iterate over it and find the largest
	// tier with base point lower or equal than current point
	memberships, err := server.listMemberships(ctx)
	if err != nil {
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
// @Success      200  {array}   db.Membership    "List of memberships retrieved successfully"
// @Failure      401  {object}  ErrorResponse     "Unauthorized access"
// @Failure      500  {object}  ErrorResponse     "Internal server error or failed to communicate with Directus"
// @Security BearerAuth
// @Router       /api/memberships [get]
func (server *Server) ListMemberships(ctx *gin.Context) {
	// Get the list of all memberships tier
	memberships, err := server.listMemberships(ctx)
	if err != nil {
		// The helper already handle error return to client, so we just return here
		return
	}
	ctx.JSON(http.StatusOK, memberships)
}

// Helper method: Get the list of all memberships
func (server *Server) listMemberships(ctx *gin.Context) ([]db.Membership, error) {
	// Get access token
	token := server.GetToken(ctx)

	// Get the list of all memberships. It should be a short list, so we don't need to provide any paging here
	url := fmt.Sprintf("%s/items/memberships?filter[status][_eq]=published&sort=base_point", server.config.DirectusAddr)
	var memberships = []db.Membership{} // Make sure it's an empty slice instead of nil for better JSON returned
	status, err := db.MakeRequest("GET", url, nil, token, &memberships)
	if err != nil {
		util.LOGGER.Error(
			fmt.Sprintf("%s %s: failed to get the list of all memberships", ctx.Request.Method, ctx.FullPath()),
			"status", status,
			"error", err,
		)
		server.DirectusError(ctx, err)
		return nil, err
	}

	return memberships, nil
}
