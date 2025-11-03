package api

import (
	"fmt"
	"net/http"
	"net/url"
	"tekticket/service/payment"
	"tekticket/util"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v82"
)

// ============ Request/Response Models ============

// Request to create a booking
type CreateBookingRequest struct {
	EventID string              `json:"event_id" binding:"required"`
	Items   []BookingItemCreate `json:"items" binding:"required,min=1,dive"`
}

type BookingItemCreate struct {
	TicketID string `json:"ticket_id" binding:"required"`
	SeatID   string `json:"seat_id" binding:"required"`
}

// Response for booking creation
type CreateBookingResponse struct {
	BookingID    string        `json:"booking_id"`
	Status       string        `json:"status"`
	Items        []BookingItem `json:"items"`
	TotalAmount  int           `json:"total_amount"`
	PaymentFee   int           `json:"payment_fee"`
	FinalAmount  int           `json:"final_amount"`
	ExpiresAt    string        `json:"expires_at"`
	PaymentID    string        `json:"payment_id"`
	ClientSecret string        `json:"client_secret"`
	Currency     string        `json:"currency"`
}

type BookingItem struct {
	ID       string `json:"id"`
	TicketID string `json:"ticket_id"`
	SeatID   string `json:"seat_id"`
	Price    int    `json:"price"`
	Rank     string `json:"rank"`
}

// Request to confirm payment
type ConfirmPaymentRequest struct {
	PaymentID     string `json:"payment_id" binding:"required"`
	PaymentMethod string `json:"payment_method" binding:"required"`
	Status        string `json:"status" binding:"required"` // Status from Stripe (succeeded, failed, etc.)
}

// Response for payment confirmation
type ConfirmPaymentResponse struct {
	PaymentID string `json:"payment_id"`
	BookingID string `json:"booking_id"`
	Status    string `json:"status"`
	Amount    int    `json:"amount"`
}

// Get booking details response
type BookingDetailResponse struct {
	ID          string              `json:"id"`
	EventID     string              `json:"event_id"`
	EventName   string              `json:"event_name"`
	Status      string              `json:"status"`
	Items       []BookingItemDetail `json:"items"`
	TotalAmount int                 `json:"total_amount"`
	CreatedAt   string              `json:"created_at"`
	ExpiresAt   string              `json:"expires_at"`
	Payment     *PaymentInfo        `json:"payment,omitempty"`
}

type BookingItemDetail struct {
	ID         string `json:"id"`
	TicketRank string `json:"ticket_rank"`
	SeatNumber string `json:"seat_number"`
	SeatZone   string `json:"seat_zone"`
	Price      int    `json:"price"`
	QRCode     string `json:"qr_code,omitempty"`
	Status     string `json:"status"`
}

type PaymentInfo struct {
	ID            string `json:"id"`
	Amount        int    `json:"amount"`
	Status        string `json:"status"`
	PaymentMethod string `json:"payment_method,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// Directus structures
type directusBooking struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	EventID    string `json:"event_id"`
	Status     string `json:"status"`
	CreatedAt  string `json:"date_created"`
	UpdatedAt  string `json:"date_updated"`
}

type directusBookingItem struct {
	ID        string `json:"id"`
	BookingID string `json:"booking_id"`
	TicketID  string `json:"ticket_id"`
	SeatID    string `json:"seat_id"`
	Price     int    `json:"price"`
	QR        string `json:"qr"`
	Status    string `json:"status"`
	CreatedAt string `json:"date_created"`
	UpdatedAt string `json:"date_updated"`
}

type directusPayment struct {
	ID             string `json:"id"`
	BookingID      string `json:"booking_id"`
	TransactionID  string `json:"transaction_id"`
	Amount         int    `json:"amount"`
	PaymentGateway string `json:"payment_gateway"`
	PaymentMethod  string `json:"payment_method"`
	Status         string `json:"status"`
	CreatedAt      string `json:"date_created"`
	UpdatedAt      string `json:"date_updated"`
}

type directusSeat struct {
	ID         string `json:"id"`
	SeatZoneID string `json:"seat_zone_id"`
	SeatNumber string `json:"seat_number"`
	Status     string `json:"status"`
	ReservedBy string `json:"reserved_by"`
}

type directusTickets struct {
	ID          string `json:"id"`
	EventID     string `json:"event_id"`
	SeatZoneID  string `json:"seat_zone_id"`
	Rank        string `json:"rank"`
	Description string `json:"description"`
	BasePrice   int    `json:"base_price"`
	Status      string `json:"status"`
}

// ============ API Handlers ============

// CreateBooking godoc
// @Summary      Create a new booking
// @Description  Creates a new booking with selected tickets and seats. Reserves seats for a limited time.
// @Tags         Bookings
// @Accept       json
// @Produce      json
// @Param        request  body      CreateBookingRequest  true  "Booking details"
// @Success      201  {object}  CreateBookingResponse  "Booking created successfully"
// @Failure      400  {object}  ErrorResponse          "Invalid request"
// @Failure      401  {object}  ErrorResponse          "Unauthorized"
// @Failure      409  {object}  ErrorResponse          "Seat already reserved or booked"
// @Failure      500  {object}  ErrorResponse          "Internal server error"
// @Security BearerAuth
// @Router       /api/bookings [post]
func (server *Server) CreateBooking(ctx *gin.Context) {
	// Get access token and extract user
	token := server.GetToken(ctx)
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Unauthorized access"})
		return
	}

	// Extract user ID from token
	userID, err := server.GetUserIDFromToken(token)
	if err != nil {
		util.LOGGER.Error("POST /api/bookings: failed to get user from token", "error", err)
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Invalid token"})
		return
	}

	// Parse request
	var req CreateBookingRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{err.Error()})
		return
	}

	// Validate that all items belong to the same event
	for _, item := range req.Items {
		// Get ticket details to verify it belongs to the event
		ticket, err := server.getTicketByID(token, item.TicketID)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, ErrorResponse{fmt.Sprintf("Invalid ticket ID: %s", item.TicketID)})
			return
		}
		if ticket.EventID != req.EventID {
			ctx.JSON(http.StatusBadRequest, ErrorResponse{"All tickets must belong to the same event"})
			return
		}
	}

	// Check seat availability and reserve seats
	for _, item := range req.Items {
		// Get seat details
		seat, err := server.getSeatByID(token, item.SeatID)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, ErrorResponse{fmt.Sprintf("Invalid seat ID: %s", item.SeatID)})
			return
		}

		// Check if seat is available
		if seat.Status != "empty" {
			ctx.JSON(http.StatusConflict, ErrorResponse{fmt.Sprintf("Seat %s is already reserved or booked", seat.SeatNumber)})
			return
		}

		// Verify seat belongs to ticket's seat zone
		ticket, _ := server.getTicketByID(token, item.TicketID)
		if seat.SeatZoneID != ticket.SeatZoneID {
			ctx.JSON(http.StatusBadRequest, ErrorResponse{fmt.Sprintf("Seat %s does not belong to ticket's seat zone", seat.SeatNumber)})
			return
		}

		// Reserve the seat
		err = server.updateSeatStatus(token, item.SeatID, "reserved", userID)
		if err != nil {
			util.LOGGER.Error("POST /api/bookings: failed to reserve seat", "error", err, "seat_id", item.SeatID)
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Failed to reserve seat"})
			return
		}
	}

	// Create booking record
	bookingID := uuid.New().String()
	bookingData := map[string]any{
		"id":          bookingID,
		"customer_id": userID,
		"event_id":    req.EventID,
		"status":      "pending",
	}

	directusURL := fmt.Sprintf("%s/items/bookings", server.config.DirectusAddr)
	var bookingResult directusBooking
	statusCode, err := util.MakeRequest("POST", directusURL, bookingData, token, &bookingResult)
	if err != nil {
		util.LOGGER.Error("POST /api/bookings: failed to create booking", "error", err)
		// Rollback: release reserved seats
		server.releaseSeats(token, req.Items)
		ctx.JSON(statusCode, ErrorResponse{err.Error()})
		return
	}

	// Create booking items and calculate total
	bookingItems := make([]BookingItem, 0)
	totalAmount := 0

	for _, item := range req.Items {
		// Get ticket to calculate price
		ticket, _ := server.getTicketByID(token, item.TicketID)

		// Calculate price with membership discount
		price := server.calculatePriceWithDiscount(token, userID, ticket.BasePrice)
		totalAmount += price

		// Create booking item
		itemID := uuid.New().String()

		// Generate QR code immediately to avoid null unique constraint violation
		qrContent, err := server.generateQRContent(itemID)
		if err != nil {
			util.LOGGER.Error("POST /api/bookings: failed to generate QR content", "error", err, "item_id", itemID)
			// Use a unique placeholder if QR generation fails
			qrContent = fmt.Sprintf("pending_%s", itemID)
		}

		itemData := map[string]any{
			"id":         itemID,
			"booking_id": bookingID,
			"ticket_id":  item.TicketID,
			"seat_id":    item.SeatID,
			"price":      price,
			"qr":         qrContent,
			"status":     "valid",
		}

		directusURL := fmt.Sprintf("%s/items/booking_items", server.config.DirectusAddr)
		var itemResult directusBookingItem
		statusCode, err := util.MakeRequest("POST", directusURL, itemData, token, &itemResult)
		if err != nil {
			util.LOGGER.Error("POST /api/bookings: failed to create booking item", "error", err)
			// Rollback: delete booking and release seats
			server.deleteBooking(token, bookingID)
			server.releaseSeats(token, req.Items)
			ctx.JSON(statusCode, ErrorResponse{err.Error()})
			return
		}

		bookingItems = append(bookingItems, BookingItem{
			ID:       itemID,
			TicketID: item.TicketID,
			SeatID:   item.SeatID,
			Price:    price,
			Rank:     ticket.Rank,
		})
	}

	// Get payment fee percent from settings
	settings, err := server.getSystemSettings(token)
	if err != nil {
		util.LOGGER.Error("POST /api/bookings: failed to get system settings", "error", err)
		// Rollback
		server.deleteBooking(token, bookingID)
		server.releaseSeats(token, req.Items)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Failed to get system settings"})
		return
	}

	// Calculate final amount with payment fee
	paymentFee := float64(totalAmount) * settings.PaymentFeePercent / 100
	finalAmount := totalAmount + int(paymentFee)

	// Initialize Stripe with secret key
	payment.InitStripe(server.config.StripeSecretKey)

	// Create payment intent with Stripe
	paymentIntent, err := payment.CreatePaymentIntent(int64(finalAmount), stripe.CurrencyVND)
	if err != nil {
		util.LOGGER.Error("POST /api/bookings: failed to create payment intent", "error", err)
		// Rollback
		server.deleteBooking(token, bookingID)
		server.releaseSeats(token, req.Items)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Failed to create payment intent"})
		return
	}

	// Save payment record
	paymentID := uuid.New().String()
	paymentData := map[string]any{
		"id":              paymentID,
		"booking_id":      bookingID,
		"transaction_id":  paymentIntent.ID,
		"amount":          finalAmount,
		"payment_gateway": "Stripe",
		"payment_method":  "pending",
		"status":          "pending",
	}

	directusURL = fmt.Sprintf("%s/items/payments", server.config.DirectusAddr)
	var paymentResult directusPayment
	statusCode, err = util.MakeRequest("POST", directusURL, paymentData, token, &paymentResult)
	if err != nil {
		util.LOGGER.Error("POST /api/bookings: failed to save payment", "error", err)
		// Rollback
		server.deleteBooking(token, bookingID)
		server.releaseSeats(token, req.Items)
		ctx.JSON(statusCode, ErrorResponse{err.Error()})
		return
	}

	// Calculate expiration time
	maxHoldMinutes := settings.MaxReservationHoldMinutes
	expiresAt := time.Now().Add(time.Duration(maxHoldMinutes) * time.Minute)

	util.LOGGER.Info("POST /api/bookings: booking created successfully",
		"booking_id", bookingID,
		"payment_id", paymentID,
		"total_amount", totalAmount,
		"payment_fee", int(paymentFee),
		"final_amount", finalAmount)

	response := CreateBookingResponse{
		BookingID:    bookingID,
		Status:       "pending",
		Items:        bookingItems,
		TotalAmount:  totalAmount,
		PaymentFee:   int(paymentFee),
		FinalAmount:  finalAmount,
		ExpiresAt:    expiresAt.Format(time.RFC3339),
		PaymentID:    paymentID,
		ClientSecret: paymentIntent.ClientSecret,
		Currency:     string(stripe.CurrencyVND),
	}

	ctx.JSON(http.StatusCreated, response)
}

// ConfirmPayment godoc
// @Summary      Confirm payment status
// @Description  Updates payment and booking status based on Stripe payment result
// @Tags         Bookings
// @Accept       json
// @Produce      json
// @Param        request  body      ConfirmPaymentRequest  true  "Payment confirmation"
// @Success      200  {object}  ConfirmPaymentResponse  "Payment confirmed"
// @Failure      400  {object}  ErrorResponse           "Invalid request"
// @Failure      401  {object}  ErrorResponse           "Unauthorized"
// @Failure      404  {object}  ErrorResponse           "Payment not found"
// @Failure      500  {object}  ErrorResponse           "Internal server error"
// @Security BearerAuth
// @Router       /api/bookings/confirm-payment [post]
func (server *Server) ConfirmPayment(ctx *gin.Context) {
	// Get access token
	token := server.GetToken(ctx)
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Unauthorized access"})
		return
	}

	// Get user ID
	userID, err := server.GetUserIDFromToken(token)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Invalid token"})
		return
	}

	// Parse request
	var req ConfirmPaymentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{err.Error()})
		return
	}

	// Get payment details
	paymentRecord, err := server.getPaymentByID(token, req.PaymentID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, ErrorResponse{"Payment not found"})
		return
	}

	// Get booking to verify ownership
	booking, err := server.getBookingByID(token, paymentRecord.BookingID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, ErrorResponse{"Booking not found"})
		return
	}

	// Verify booking belongs to user
	if booking.CustomerID != userID {
		ctx.JSON(http.StatusForbidden, ErrorResponse{"You don't have permission to confirm this payment"})
		return
	}

	// Initialize Stripe
	payment.InitStripe(server.config.StripeSecretKey)

	// Confirm payment intent with Stripe
	returnURL := fmt.Sprintf("%s/payment/confirm", server.config.FrontendURL)
	paymentIntent, err := payment.ConfirmPaymentIntent(
		paymentRecord.TransactionID,
		req.PaymentMethod,
		returnURL,
	)
	if err != nil {
		util.LOGGER.Error("POST /api/bookings/confirm-payment: failed to confirm payment intent",
			"error", err,
			"payment_id", req.PaymentID,
			"transaction_id", paymentRecord.TransactionID)

		// Update payment status to failed
		updateData := map[string]any{
			"status":         "failed",
			"payment_method": req.PaymentMethod,
		}
		directusURL := fmt.Sprintf("%s/items/payments/%s", server.config.DirectusAddr, req.PaymentID)
		util.MakeRequest("PATCH", directusURL, updateData, token, &map[string]any{})

		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Failed to confirm payment with Stripe"})
		return
	}

	// Check payment status from Stripe response
	stripeStatus := string(paymentIntent.Status)

	if stripeStatus == "succeeded" && req.Status == "succeeded" {
		// Payment success - update payment status to success
		updatePaymentData := map[string]any{
			"status":         "success",
			"payment_method": req.PaymentMethod,
		}
		directusURL := fmt.Sprintf("%s/items/payments/%s", server.config.DirectusAddr, req.PaymentID)
		_, err = util.MakeRequest("PATCH", directusURL, updatePaymentData, token, &map[string]any{})
		if err != nil {
			util.LOGGER.Error("POST /api/bookings/confirm-payment: failed to update payment status", "error", err)
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Failed to update payment status"})
			return
		}

		// Update booking status to complete
		updateBookingData := map[string]any{
			"status": "complete",
		}
		bookingURL := fmt.Sprintf("%s/items/bookings/%s", server.config.DirectusAddr, paymentRecord.BookingID)
		_, err = util.MakeRequest("PATCH", bookingURL, updateBookingData, token, &map[string]any{})
		if err != nil {
			util.LOGGER.Error("POST /api/bookings/confirm-payment: failed to update booking status", "error", err)
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Failed to update booking status"})
			return
		}

		// Update seat status from reserved to booked
		bookingItems, err := server.getBookingItems(token, paymentRecord.BookingID)
		if err == nil {
			for _, item := range bookingItems {
				server.updateSeatStatus(token, item.SeatID, "booked", "")
			}
		}

		util.LOGGER.Info("POST /api/bookings/confirm-payment: payment successful",
			"payment_id", req.PaymentID,
			"booking_id", paymentRecord.BookingID,
			"transaction_id", paymentRecord.TransactionID,
			"stripe_status", stripeStatus)

		response := ConfirmPaymentResponse{
			PaymentID: req.PaymentID,
			BookingID: paymentRecord.BookingID,
			Status:    "success",
			Amount:    paymentRecord.Amount,
		}
		ctx.JSON(http.StatusOK, response)
		return
	}

	// Payment failed
	if req.Status == "failed" || stripeStatus == "canceled" || stripeStatus == "failed" {
		// Update payment status to failed
		updateData := map[string]any{
			"status":         "failed",
			"payment_method": req.PaymentMethod,
		}
		directusURL := fmt.Sprintf("%s/items/payments/%s", server.config.DirectusAddr, req.PaymentID)
		_, err = util.MakeRequest("PATCH", directusURL, updateData, token, &map[string]any{})
		if err != nil {
			util.LOGGER.Error("POST /api/bookings/confirm-payment: failed to update payment status", "error", err)
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Failed to update payment status"})
			return
		}

		util.LOGGER.Info("POST /api/bookings/confirm-payment: payment failed",
			"payment_id", req.PaymentID,
			"status", req.Status,
			"stripe_status", stripeStatus)

		response := ConfirmPaymentResponse{
			PaymentID: req.PaymentID,
			BookingID: paymentRecord.BookingID,
			Status:    "failed",
			Amount:    paymentRecord.Amount,
		}
		ctx.JSON(http.StatusOK, response)
		return
	}

	// Payment is in other status (processing, requires_action, requires_payment_method, etc.)
	util.LOGGER.Info("POST /api/bookings/confirm-payment: payment in progress",
		"payment_id", req.PaymentID,
		"status", req.Status,
		"stripe_status", stripeStatus)

	response := ConfirmPaymentResponse{
		PaymentID: req.PaymentID,
		BookingID: paymentRecord.BookingID,
		Status:    stripeStatus,
		Amount:    paymentRecord.Amount,
	}
	ctx.JSON(http.StatusOK, response)
}

// ============ Helper Functions ============

// Get user ID from JWT token
func (server *Server) GetUserIDFromToken(token string) (string, error) {
	// Make request to get current user profile
	directusURL := fmt.Sprintf("%s/users/me", server.config.DirectusAddr)
	var user struct {
		ID string `json:"id"`
	}
	_, err := util.MakeRequest("GET", directusURL, nil, token, &user)
	if err != nil {
		return "", err
	}
	return user.ID, nil
}

// Get ticket by ID
func (server *Server) getTicketByID(token, ticketID string) (*directusTickets, error) {
	directusURL := fmt.Sprintf("%s/items/tickets/%s", server.config.DirectusAddr, ticketID)
	var ticket directusTickets
	_, err := util.MakeRequest("GET", directusURL, nil, token, &ticket)
	if err != nil {
		return nil, err
	}
	return &ticket, nil
}

// Get seat by ID
func (server *Server) getSeatByID(token, seatID string) (*directusSeat, error) {
	directusURL := fmt.Sprintf("%s/items/seats/%s", server.config.DirectusAddr, seatID)
	var seat directusSeat
	_, err := util.MakeRequest("GET", directusURL, nil, token, &seat)
	if err != nil {
		return nil, err
	}
	return &seat, nil
}

// Update seat status
func (server *Server) updateSeatStatus(token, seatID, status, reservedBy string) error {
	updateData := map[string]any{
		"status": status,
	}
	if reservedBy != "" {
		updateData["reserved_by"] = reservedBy
	} else {
		updateData["reserved_by"] = nil
	}

	directusURL := fmt.Sprintf("%s/items/seats/%s", server.config.DirectusAddr, seatID)
	_, err := util.MakeRequest("PATCH", directusURL, updateData, token, &map[string]any{})
	return err
}

// Release seats
func (server *Server) releaseSeats(token string, items []BookingItemCreate) {
	for _, item := range items {
		server.updateSeatStatus(token, item.SeatID, "empty", "")
	}
}

// Get booking by ID
func (server *Server) getBookingByID(token, bookingID string) (*directusBooking, error) {
	directusURL := fmt.Sprintf("%s/items/bookings/%s", server.config.DirectusAddr, bookingID)
	var booking directusBooking
	_, err := util.MakeRequest("GET", directusURL, nil, token, &booking)
	if err != nil {
		return nil, err
	}
	return &booking, nil
}

// Get booking items by booking ID
func (server *Server) getBookingItems(token, bookingID string) ([]directusBookingItem, error) {
	queryParams := url.Values{}
	queryParams.Add("filter[booking_id][_eq]", bookingID)

	directusURL := fmt.Sprintf("%s/items/booking_items?%s", server.config.DirectusAddr, queryParams.Encode())
	var items []directusBookingItem
	_, err := util.MakeRequest("GET", directusURL, nil, token, &items)
	if err != nil {
		return nil, err
	}
	return items, nil
}

// Get payment by ID
func (server *Server) getPaymentByID(token, paymentID string) (*directusPayment, error) {
	directusURL := fmt.Sprintf("%s/items/payments/%s", server.config.DirectusAddr, paymentID)
	var payment directusPayment
	_, err := util.MakeRequest("GET", directusURL, nil, token, &payment)
	if err != nil {
		return nil, err
	}
	return &payment, nil
}

// Delete booking
func (server *Server) deleteBooking(token, bookingID string) error {
	directusURL := fmt.Sprintf("%s/items/bookings/%s", server.config.DirectusAddr, bookingID)
	_, err := util.MakeRequest("DELETE", directusURL, nil, token, &map[string]any{})
	return err
}

// Generate QR code content (encrypted booking item ID)
func (server *Server) generateQRContent(bookingItemID string) (string, error) {
	// Encrypt the booking item ID with server secret key
	secretKey := []byte(server.config.QRSecretKey)
	if len(secretKey) != 32 {
		// Pad or truncate to 32 bytes for AES-256
		padded := make([]byte, 32)
		copy(padded, secretKey)
		secretKey = padded
	}

	encrypted, err := util.Encrypt(secretKey, []byte(bookingItemID))
	if err != nil {
		return "", err
	}

	// Encode to base64 for QR code
	qrContent := util.Encode(string(encrypted))
	return qrContent, nil
}

// Calculate price with membership discount
func (server *Server) calculatePriceWithDiscount(token, userID string, basePrice int) int {
	// Get user's membership tier
	membership, err := server.getUserMembership(token, userID)
	if err != nil {
		// No membership, return base price
		return basePrice
	}

	// Apply discount
	discount := membership.Discount
	finalPrice := float64(basePrice) * (1 - discount/100)

	return int(finalPrice)
}

// Get user membership (from memberships.go or implement here)
type userMembershipInfo struct {
	Tier     string  `json:"tier"`
	Discount float64 `json:"discount"`
}

func (server *Server) getUserMembership(token, userID string) (*userMembershipInfo, error) {
	// Get user's total points from user_membership_logs
	queryParams := url.Values{}
	queryParams.Add("filter[customer_id][_eq]", userID)
	queryParams.Add("sort", "-date_created")
	queryParams.Add("limit", "1")

	directusURL := fmt.Sprintf("%s/items/user_membership_logs?%s", server.config.DirectusAddr, queryParams.Encode())

	var logs []struct {
		ResultingPoints int `json:"resulting_points"`
	}
	_, err := util.MakeRequest("GET", directusURL, nil, token, &logs)
	if err != nil || len(logs) == 0 {
		return nil, fmt.Errorf("no membership data")
	}

	userPoints := logs[0].ResultingPoints

	// Get all membership tiers
	tiersURL := fmt.Sprintf("%s/items/memberships?filter[status][_eq]=published&sort=-base_point", server.config.DirectusAddr)
	var tiers []struct {
		Tier      string  `json:"tier"`
		BasePoint int     `json:"base_point"`
		Discount  float64 `json:"discount"`
	}
	_, err = util.MakeRequest("GET", tiersURL, nil, token, &tiers)
	if err != nil {
		return nil, err
	}

	// Find appropriate tier
	for _, tier := range tiers {
		if userPoints >= tier.BasePoint {
			return &userMembershipInfo{
				Tier:     tier.Tier,
				Discount: tier.Discount,
			}, nil
		}
	}

	return nil, fmt.Errorf("no matching tier")
}

// System settings
type systemSettings struct {
	MoneyToPointRate          int     `json:"money_to_point_rate"`
	MinEventDurationMinutes   int     `json:"min_event_duration_minutes"`
	MinEventLeadDays          int     `json:"min_event_lead_days"`
	MaxReservationHoldMinutes int     `json:"max_reservation_hold_minutes"`
	MinSellingDurationMinutes int     `json:"min_selling_duration_minutes"`
	PaymentFeePercent         float64 `json:"payment_fee_percent,string"` // Parse string to float64
	MaxFullRefundHours        int     `json:"max_full_refund_hours"`
	SystemEmail               string  `json:"system_email"`
}

func (server *Server) getSystemSettings(token string) (*systemSettings, error) {
	queryParams := url.Values{}
	queryParams.Add("filter[in_used][_eq]", "true")
	queryParams.Add("filter[status][_eq]", "published")
	queryParams.Add("limit", "1")

	directusURL := fmt.Sprintf("%s/items/settings?%s", server.config.DirectusAddr, queryParams.Encode())
	var settings []systemSettings
	_, err := util.MakeRequest("GET", directusURL, nil, token, &settings)
	if err != nil || len(settings) == 0 {
		return nil, err
	}
	return &settings[0], nil
}
