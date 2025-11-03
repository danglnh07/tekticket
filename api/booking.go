package api

import (
	"context"
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
	BookingID   string        `json:"booking_id"`
	Status      string        `json:"status"`
	Items       []BookingItem `json:"items"`
	TotalAmount int           `json:"total_amount"`
	ExpiresAt   string        `json:"expires_at"`
}

type BookingItem struct {
	ID       string `json:"id"`
	TicketID string `json:"ticket_id"`
	SeatID   string `json:"seat_id"`
	Price    int    `json:"price"`
	Rank     string `json:"rank"`
}

// Request to checkout (create payment intent)
type CheckoutRequest struct {
	BookingID string `json:"booking_id" binding:"required"`
}

// Response for checkout
type CheckoutResponse struct {
	PaymentID    string `json:"payment_id"`
	ClientSecret string `json:"client_secret"`
	Amount       int    `json:"amount"`
	Currency     string `json:"currency"`
	BookingID    string `json:"booking_id"`
}

// Request to confirm payment
type ConfirmPaymentRequest struct {
	PaymentID     string `json:"payment_id" binding:"required"`
	PaymentMethod string `json:"payment_method" binding:"required"`
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

type directusSeatZones struct {
	ID          string `json:"id"`
	Description string `json:"description"`
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
		price := server.calculatePriceWithDiscount(ctx, token, userID, ticket.BasePrice)
		totalAmount += price

		// Create booking item
		itemID := uuid.New().String()
		itemData := map[string]any{
			"id":         itemID,
			"booking_id": bookingID,
			"ticket_id":  item.TicketID,
			"seat_id":    item.SeatID,
			"price":      price,
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

	// Calculate expiration time (from system settings)
	maxHoldMinutes := 5 // Default, should be fetched from settings
	settings, err := server.getSystemSettings(token)
	if err == nil && settings != nil {
		maxHoldMinutes = settings.MaxReservationHoldMinutes
	}
	expiresAt := time.Now().Add(time.Duration(maxHoldMinutes) * time.Minute)

	// Schedule background job to release seats if payment not made
	// TODO: Implement background worker to check expired bookings

	response := CreateBookingResponse{
		BookingID:   bookingID,
		Status:      "pending",
		Items:       bookingItems,
		TotalAmount: totalAmount,
		ExpiresAt:   expiresAt.Format(time.RFC3339),
	}

	ctx.JSON(http.StatusCreated, response)
}

// Checkout godoc
// @Summary      Create payment intent for booking
// @Description  Creates a Stripe payment intent for a pending booking
// @Tags         Bookings
// @Accept       json
// @Produce      json
// @Param        request  body      CheckoutRequest  true  "Checkout request"
// @Success      200  {object}  CheckoutResponse  "Payment intent created"
// @Failure      400  {object}  ErrorResponse     "Invalid request"
// @Failure      401  {object}  ErrorResponse     "Unauthorized"
// @Failure      404  {object}  ErrorResponse     "Booking not found"
// @Failure      409  {object}  ErrorResponse     "Booking already paid or expired"
// @Failure      500  {object}  ErrorResponse     "Internal server error"
// @Security BearerAuth
// @Router       /api/bookings/checkout [post]
func (server *Server) Checkout(ctx *gin.Context) {
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
	var req CheckoutRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{err.Error()})
		return
	}

	// Get booking details
	booking, err := server.getBookingByID(token, req.BookingID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, ErrorResponse{"Booking not found"})
		return
	}

	// Verify booking belongs to user
	if booking.CustomerID != userID {
		ctx.JSON(http.StatusForbidden, ErrorResponse{"You don't have permission to checkout this booking"})
		return
	}

	// Check booking status
	if booking.Status != "pending" {
		ctx.JSON(http.StatusConflict, ErrorResponse{fmt.Sprintf("Booking is %s, cannot checkout", booking.Status)})
		return
	}

	// Get booking items to calculate total amount
	bookingItems, err := server.getBookingItems(token, req.BookingID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Failed to get booking items"})
		return
	}

	// Calculate total amount with payment fee
	subtotal := 0
	for _, item := range bookingItems {
		subtotal += item.Price
	}

	// Apply payment fee (from system settings)
	paymentFeePercent := 5.0 // Default
	settings, err := server.getSystemSettings(token)
	if err == nil && settings != nil {
		paymentFeePercent = settings.PaymentFeePercent
	}
	totalAmount := int(float64(subtotal) * (1 + paymentFeePercent/100))

	// Create Stripe payment intent
	intent, err := payment.CreatePaymentIntent(int64(totalAmount), stripe.CurrencyVND)
	if err != nil {
		util.LOGGER.Error("POST /api/bookings/checkout: failed to create payment intent", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Failed to create payment intent"})
		return
	}

	// Save payment record
	paymentID := uuid.New().String()
	paymentData := map[string]any{
		"id":              paymentID,
		"booking_id":      req.BookingID,
		"transaction_id":  intent.ID,
		"amount":          totalAmount,
		"payment_gateway": "Stripe",
		"payment_method":  "pending",
		"status":          "pending",
	}

	directusURL := fmt.Sprintf("%s/items/payments", server.config.DirectusAddr)
	var paymentResult directusPayment
	statusCode, err := util.MakeRequest("POST", directusURL, paymentData, token, &paymentResult)
	if err != nil {
		util.LOGGER.Error("POST /api/bookings/checkout: failed to save payment", "error", err)
		ctx.JSON(statusCode, ErrorResponse{err.Error()})
		return
	}

	response := CheckoutResponse{
		PaymentID:    paymentID,
		ClientSecret: intent.ClientSecret,
		Amount:       totalAmount,
		Currency:     string(stripe.CurrencyVND),
		BookingID:    req.BookingID,
	}

	ctx.JSON(http.StatusOK, response)
}

// ConfirmPayment godoc
// @Summary      Confirm payment for booking
// @Description  Confirms payment with Stripe and completes the booking
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

	// Parse request
	var req ConfirmPaymentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{err.Error()})
		return
	}

	// Get payment record
	paymentRecord, err := server.getPaymentByID(token, req.PaymentID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, ErrorResponse{"Payment not found"})
		return
	}

	// Confirm payment with Stripe
	returnURL := fmt.Sprintf("%s/booking-confirmation", server.config.FrontendURL)
	intent, err := payment.ConfirmPaymentIntent(paymentRecord.TransactionID, req.PaymentMethod, returnURL)
	if err != nil {
		util.LOGGER.Error("POST /api/bookings/confirm-payment: failed to confirm payment", "error", err)

		// Update payment status to failed
		server.updatePaymentStatus(token, req.PaymentID, "failed", req.PaymentMethod)

		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Failed to confirm payment"})
		return
	}

	// Map Stripe status to our status
	paymentStatus := server.mapStripeStatus(string(intent.Status))

	// Update payment record
	err = server.updatePaymentStatus(token, req.PaymentID, paymentStatus, req.PaymentMethod)
	if err != nil {
		util.LOGGER.Error("POST /api/bookings/confirm-payment: failed to update payment", "error", err)
	}

	// If payment successful, complete the booking
	if paymentStatus == "success" {
		err = server.completeBooking(ctx, token, paymentRecord.BookingID)
		if err != nil {
			util.LOGGER.Error("POST /api/bookings/confirm-payment: failed to complete booking", "error", err)
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Payment successful but booking completion failed"})
			return
		}
	}

	response := ConfirmPaymentResponse{
		PaymentID: req.PaymentID,
		BookingID: paymentRecord.BookingID,
		Status:    paymentStatus,
		Amount:    paymentRecord.Amount,
	}

	ctx.JSON(http.StatusOK, response)
}

// GetBooking godoc
// @Summary      Get booking details
// @Description  Retrieves detailed information about a booking
// @Tags         Bookings
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Booking ID"
// @Success      200  {object}  BookingDetailResponse  "Booking details"
// @Failure      401  {object}  ErrorResponse          "Unauthorized"
// @Failure      404  {object}  ErrorResponse          "Booking not found"
// @Failure      500  {object}  ErrorResponse          "Internal server error"
// @Security BearerAuth
// @Router       /api/bookings/{id} [get]
func (server *Server) GetBooking(ctx *gin.Context) {
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

	// Get booking ID from path
	bookingID := ctx.Param("id")
	if bookingID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Booking ID is required"})
		return
	}

	// Get booking
	booking, err := server.getBookingByID(token, bookingID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, ErrorResponse{"Booking not found"})
		return
	}

	// Verify ownership
	if booking.CustomerID != userID {
		ctx.JSON(http.StatusForbidden, ErrorResponse{"You don't have permission to view this booking"})
		return
	}

	// Get event details
	event, err := server.getEventByID(token, booking.EventID)
	if err != nil {
		util.LOGGER.Error("GET /api/bookings/:id: failed to get event", "error", err)
	}

	// Get booking items with details
	items, err := server.getBookingItemsWithDetails(token, bookingID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Failed to get booking items"})
		return
	}

	// Calculate total
	totalAmount := 0
	for _, item := range items {
		totalAmount += item.Price
	}

	// Get payment info if exists
	var paymentInfo *PaymentInfo
	payments, err := server.getPaymentsByBookingID(token, bookingID)
	if err == nil && len(payments) > 0 {
		// Get the latest payment
		latestPayment := payments[len(payments)-1]
		paymentInfo = &PaymentInfo{
			ID:            latestPayment.ID,
			Amount:        latestPayment.Amount,
			Status:        latestPayment.Status,
			PaymentMethod: latestPayment.PaymentMethod,
			CreatedAt:     latestPayment.CreatedAt,
		}
	}

	// Calculate expiration
	maxHoldMinutes := 5
	settings, err := server.getSystemSettings(token)
	if err == nil && settings != nil {
		maxHoldMinutes = settings.MaxReservationHoldMinutes
	}

	createdTime, _ := time.Parse(time.RFC3339, booking.CreatedAt)
	expiresAt := createdTime.Add(time.Duration(maxHoldMinutes) * time.Minute)

	response := BookingDetailResponse{
		ID:          booking.ID,
		EventID:     booking.EventID,
		EventName:   event.Name,
		Status:      booking.Status,
		Items:       items,
		TotalAmount: totalAmount,
		CreatedAt:   booking.CreatedAt,
		ExpiresAt:   expiresAt.Format(time.RFC3339),
		Payment:     paymentInfo,
	}

	ctx.JSON(http.StatusOK, response)
}

// ListMyBookings godoc
// @Summary      List user's bookings
// @Description  Retrieves all bookings for the authenticated user
// @Tags         Bookings
// @Accept       json
// @Produce      json
// @Param        status  query     string  false  "Filter by status (pending, complete, canceled, timeout)"
// @Param        limit   query     int     false  "Limit results (default: 50)"
// @Param        offset  query     int     false  "Offset for pagination (default: 0)"
// @Success      200  {array}   BookingDetailResponse  "List of bookings"
// @Failure      401  {object}  ErrorResponse          "Unauthorized"
// @Failure      500  {object}  ErrorResponse          "Internal server error"
// @Security BearerAuth
// @Router       /api/bookings [get]
func (server *Server) ListMyBookings(ctx *gin.Context) {
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

	// Build query parameters
	queryParams := url.Values{}
	queryParams.Add("filter[customer_id][_eq]", userID)

	// Filter by status if provided
	if status := ctx.Query("status"); status != "" {
		queryParams.Add("filter[status][_eq]", status)
	}

	// Pagination
	limit := 50
	if limitStr := ctx.Query("limit"); limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}
	queryParams.Add("limit", fmt.Sprintf("%d", limit))

	offset := 0
	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		fmt.Sscanf(offsetStr, "%d", &offset)
	}
	queryParams.Add("offset", fmt.Sprintf("%d", offset))

	// Sort by creation date (newest first)
	queryParams.Add("sort", "-date_created")

	// Make request
	directusURL := fmt.Sprintf("%s/items/bookings?%s", server.config.DirectusAddr, queryParams.Encode())
	var bookings []directusBooking
	statusCode, err := util.MakeRequest("GET", directusURL, nil, token, &bookings)
	if err != nil {
		util.LOGGER.Error("GET /api/bookings: failed to get bookings", "error", err)
		ctx.JSON(statusCode, ErrorResponse{err.Error()})
		return
	}

	// Build response
	response := make([]BookingDetailResponse, 0)
	for _, booking := range bookings {
		// Get event
		event, _ := server.getEventByID(token, booking.EventID)

		// Get items
		items, _ := server.getBookingItemsWithDetails(token, booking.ID)

		// Calculate total
		totalAmount := 0
		for _, item := range items {
			totalAmount += item.Price
		}

		// Get payment
		var paymentInfo *PaymentInfo
		payments, err := server.getPaymentsByBookingID(token, booking.ID)
		if err == nil && len(payments) > 0 {
			latestPayment := payments[len(payments)-1]
			paymentInfo = &PaymentInfo{
				ID:            latestPayment.ID,
				Amount:        latestPayment.Amount,
				Status:        latestPayment.Status,
				PaymentMethod: latestPayment.PaymentMethod,
				CreatedAt:     latestPayment.CreatedAt,
			}
		}

		// Calculate expiration
		maxHoldMinutes := 5
		settings, _ := server.getSystemSettings(token)
		if settings != nil {
			maxHoldMinutes = settings.MaxReservationHoldMinutes
		}

		createdTime, _ := time.Parse(time.RFC3339, booking.CreatedAt)
		expiresAt := createdTime.Add(time.Duration(maxHoldMinutes) * time.Minute)

		response = append(response, BookingDetailResponse{
			ID:          booking.ID,
			EventID:     booking.EventID,
			EventName:   event.Name,
			Status:      booking.Status,
			Items:       items,
			TotalAmount: totalAmount,
			CreatedAt:   booking.CreatedAt,
			ExpiresAt:   expiresAt.Format(time.RFC3339),
			Payment:     paymentInfo,
		})
	}

	ctx.JSON(http.StatusOK, response)
}

// CancelBooking godoc
// @Summary      Cancel a booking
// @Description  Cancels a pending booking and releases reserved seats
// @Tags         Bookings
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Booking ID"
// @Success      200  {object}  SuccessMessage  "Booking canceled successfully"
// @Failure      400  {object}  ErrorResponse   "Booking cannot be canceled"
// @Failure      401  {object}  ErrorResponse   "Unauthorized"
// @Failure      404  {object}  ErrorResponse   "Booking not found"
// @Failure      500  {object}  ErrorResponse   "Internal server error"
// @Security BearerAuth
// @Router       /api/bookings/{id}/cancel [post]
func (server *Server) CancelBooking(ctx *gin.Context) {
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

	// Get booking ID
	bookingID := ctx.Param("id")
	if bookingID == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Booking ID is required"})
		return
	}

	// Get booking
	booking, err := server.getBookingByID(token, bookingID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, ErrorResponse{"Booking not found"})
		return
	}

	// Verify ownership
	if booking.CustomerID != userID {
		ctx.JSON(http.StatusForbidden, ErrorResponse{"You don't have permission to cancel this booking"})
		return
	}

	// Check if booking can be canceled
	if booking.Status != "pending" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Only pending bookings can be canceled"})
		return
	}

	// Update booking status
	updateData := map[string]any{
		"status": "canceled",
	}
	directusURL := fmt.Sprintf("%s/items/bookings/%s", server.config.DirectusAddr, bookingID)
	statusCode, err := util.MakeRequest("PATCH", directusURL, updateData, token, &map[string]any{})
	if err != nil {
		util.LOGGER.Error("POST /api/bookings/:id/cancel: failed to update booking", "error", err)
		ctx.JSON(statusCode, ErrorResponse{err.Error()})
		return
	}

	// Release all seats
	items, _ := server.getBookingItems(token, bookingID)
	for _, item := range items {
		server.updateSeatStatus(token, item.SeatID, "empty", "")
	}

	ctx.JSON(http.StatusOK, SuccessMessage{"Booking canceled successfully"})
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

// Get booking items
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

// Get booking items with details (ticket, seat info)
func (server *Server) getBookingItemsWithDetails(token, bookingID string) ([]BookingItemDetail, error) {
	items, err := server.getBookingItems(token, bookingID)
	if err != nil {
		return nil, err
	}

	result := make([]BookingItemDetail, 0)
	for _, item := range items {
		// Get ticket
		ticket, err := server.getTicketByID(token, item.TicketID)
		if err != nil {
			continue
		}

		// Get seat
		seat, err := server.getSeatByID(token, item.SeatID)
		if err != nil {
			continue
		}

		// Get seat zone
		seatZone, err := server.getSeatZoneByID(token, seat.SeatZoneID)
		if err != nil {
			continue
		}

		result = append(result, BookingItemDetail{
			ID:         item.ID,
			TicketRank: ticket.Rank,
			SeatNumber: seat.SeatNumber,
			SeatZone:   seatZone.Description,
			Price:      item.Price,
			QRCode:     item.QR,
			Status:     item.Status,
		})
	}

	return result, nil
}

// Get seat zone by ID
func (server *Server) getSeatZoneByID(token, seatZoneID string) (*directusSeatZones, error) {
	directusURL := fmt.Sprintf("%s/items/seat_zones/%s", server.config.DirectusAddr, seatZoneID)
	var seatZone directusSeatZones
	_, err := util.MakeRequest("GET", directusURL, nil, token, &seatZone)
	if err != nil {
		return nil, err
	}
	return &seatZone, nil
}

// Get event by ID (minimal info)
type directusEventMinimal struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (server *Server) getEventByID(token, eventID string) (*directusEventMinimal, error) {
	directusURL := fmt.Sprintf("%s/items/events/%s?fields=id,name", server.config.DirectusAddr, eventID)
	var event directusEventMinimal
	_, err := util.MakeRequest("GET", directusURL, nil, token, &event)
	if err != nil {
		return nil, err
	}
	return &event, nil
}

// Delete booking
func (server *Server) deleteBooking(token, bookingID string) error {
	directusURL := fmt.Sprintf("%s/items/bookings/%s", server.config.DirectusAddr, bookingID)
	_, err := util.MakeRequest("DELETE", directusURL, nil, token, &map[string]any{})
	return err
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

// Get payments by booking ID
func (server *Server) getPaymentsByBookingID(token, bookingID string) ([]directusPayment, error) {
	queryParams := url.Values{}
	queryParams.Add("filter[booking_id][_eq]", bookingID)
	queryParams.Add("sort", "date_created")

	directusURL := fmt.Sprintf("%s/items/payments?%s", server.config.DirectusAddr, queryParams.Encode())
	var payments []directusPayment
	_, err := util.MakeRequest("GET", directusURL, nil, token, &payments)
	if err != nil {
		return nil, err
	}
	return payments, nil
}

// Update payment status
func (server *Server) updatePaymentStatus(token, paymentID, status, paymentMethod string) error {
	updateData := map[string]any{
		"status": status,
	}
	if paymentMethod != "" && paymentMethod != "pending" {
		updateData["payment_method"] = paymentMethod
	}

	directusURL := fmt.Sprintf("%s/items/payments/%s", server.config.DirectusAddr, paymentID)
	_, err := util.MakeRequest("PATCH", directusURL, updateData, token, &map[string]any{})
	return err
}

// Map Stripe payment status to our status
func (server *Server) mapStripeStatus(stripeStatus string) string {
	switch stripeStatus {
	case "succeeded":
		return "success"
	case "processing":
		return "pending"
	case "requires_payment_method", "requires_confirmation", "requires_action":
		return "pending"
	case "canceled":
		return "failed"
	default:
		return "failed"
	}
}

// Complete booking after successful payment
func (server *Server) completeBooking(ctx context.Context, token, bookingID string) error {
	// Update booking status to complete
	updateData := map[string]any{
		"status": "complete",
	}
	directusURL := fmt.Sprintf("%s/items/bookings/%s", server.config.DirectusAddr, bookingID)
	_, err := util.MakeRequest("PATCH", directusURL, updateData, token, &map[string]any{})
	if err != nil {
		return err
	}

	// Update all seats to booked
	items, err := server.getBookingItems(token, bookingID)
	if err != nil {
		return err
	}

	for _, item := range items {
		// Update seat status
		err := server.updateSeatStatus(token, item.SeatID, "booked", "")
		if err != nil {
			util.LOGGER.Error("completeBooking: failed to update seat", "error", err, "seat_id", item.SeatID)
		}

		// Generate QR code for each booking item
		qrContent, err := server.generateQRContent(item.ID)
		if err != nil {
			util.LOGGER.Error("completeBooking: failed to generate QR", "error", err, "item_id", item.ID)
			continue
		}

		// Update booking item with QR
		updateItemData := map[string]any{
			"qr": qrContent,
		}
		itemURL := fmt.Sprintf("%s/items/booking_items/%s", server.config.DirectusAddr, item.ID)
		_, err = util.MakeRequest("PATCH", itemURL, updateItemData, token, &map[string]any{})
		if err != nil {
			util.LOGGER.Error("completeBooking: failed to save QR", "error", err, "item_id", item.ID)
		}
	}

	// Update user points (earn points from payment)
	// TODO: Implement user_membership_logs update

	return nil
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
func (server *Server) calculatePriceWithDiscount(ctx *gin.Context, token, userID string, basePrice int) int {
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
	PaymentFeePercent         float64 `json:"payment_fee_percent"`
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
