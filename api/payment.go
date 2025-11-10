package api

import (
	"fmt"
	"net/http"
	"strings"
	"tekticket/db"
	"tekticket/service/payment"
	"tekticket/service/worker"
	"tekticket/util"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/stripe/stripe-go/v82"
)

// Create a new payment record with status = pending
func (server *Server) createPaymentRecord(token string, amount int64, bookingID string) (string, error) {
	url := fmt.Sprintf("%s/items/payments?fields=id", server.config.DirectusAddr)
	body := map[string]any{
		"amount":     amount,
		"booking_id": bookingID,
		"status":     "pending",
	}
	var paymentInfo db.Payment
	status, err := db.MakeRequest("POST", url, body, token, &paymentInfo)
	if err != nil {
		return "", fmt.Errorf("failed to create payment record (%d): %w", status, err)
	}

	return paymentInfo.ID, nil
}

// Check if a payment with ID is valid for create payment (ID exists and status = failed)
func (server *Server) isPaymentValid(token string, paymentID string) (bool, error) {
	url := fmt.Sprintf("%s/items/payments/%s?fields=id,status", server.config.DirectusAddr, paymentID)
	var paymentInfo db.Payment
	status, err := db.MakeRequest("GET", url, nil, token, &paymentInfo)
	if err != nil {
		return false, err
	}

	// If payment found, but its status is not failed, then we still consider it invalid, and return false
	return status == http.StatusOK && paymentInfo.Status == "failed", nil
}

// Update payment
func (server *Server) updatePayment(token, paymentID string, body map[string]any) (*db.Payment, error) {
	fields := []string{
		"id", "transaction_id", "amount", "payment_gateway", "status", "booking_id.id", "date_created",
	}
	url := fmt.Sprintf("%s/items/payments/%s?fields=%s", server.config.DirectusAddr, paymentID, strings.Join(fields, ","))
	var paymentInfo db.Payment
	statusCode, err := db.MakeRequest("PATCH", url, body, token, &paymentInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to update payment (%d): %w", statusCode, err)
	}

	return &paymentInfo, nil
}

type CreatePaymentRequest struct {
	BookingID string `json:"booking_id" binding:"required"`
	Amount    int64  `json:"amount" binding:"required"`
	PaymentID string `json:"payment_id"` // This is used for retry when payment failed
}

type CreatePaymentResponse struct {
	Payment        db.Payment `json:"payment"`
	ClientSecret   string     `json:"client_secret"`
	PublishableKey string     `json:"publishable_key"`
}

type CreatePaymentError struct {
	Message   string `json:"message"`
	PaymentID string `json:"payment_id,omitempty"`
}

// CreatePayment godoc
// @Summary      Create or retry a Stripe payment
// @Description  Creates a new payment intent in Stripe and records it in Directus.
// @Description  If a `payment_id` is provided, retries the payment only if the existing record’s status is `failed`.
// @Description  Validates amount range for VND, creates a Stripe payment intent with idempotency protection, and updates Directus with transaction details.
// @Tags         Payments
// @Accept       json
// @Produce      json
// @Param        request  body  CreatePaymentRequest  true  "Payment creation payload"
// @Success      200  {object}  CreatePaymentResponse  "Payment intent successfully created"
// @Failure      400  {object}  CreatePaymentError     "Invalid request body or parameters"
// @Failure      401  {object}  CreatePaymentError     "Unauthorized access"
// @Failure      500  {object}  CreatePaymentError     "Internal server error or failed Stripe/Directus operation"
// @Security     BearerAuth
// @Router       /api/payments [post]
func (server *Server) CreatePayment(ctx *gin.Context) {
	// Get access token
	token := server.GetToken(ctx)
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, CreatePaymentError{Message: "Unauthorized access"})
		return
	}

	// Get request body
	var req CreatePaymentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Error("POST /api/payments: failed to parse request body", "error", err)
		ctx.JSON(http.StatusBadRequest, CreatePaymentError{Message: "Invalid request body"})
		return
	}

	// Check if amount is valid. For VND currency, Stripe only accept amount in range [100, 99_999_999]
	if req.Amount < 100 || req.Amount > 99_999_999 {
		ctx.JSON(http.StatusBadRequest, CreatePaymentError{Message: "Payment amount must be in range [100, 99.999.999] for VND"})
		return
	}

	// If req.PaymentID not provided -> first try, we create a new payment record
	if req.PaymentID == "" {
		paymentID, err := server.createPaymentRecord(token, req.Amount, req.BookingID)
		if err != nil {
			util.LOGGER.Error("POST /api/payments: failed to create payment record for first try payment", "error", err)
			ctx.JSON(http.StatusInternalServerError, CreatePaymentError{Message: "Internal server error"})
			return
		}
		req.PaymentID = paymentID
	} else {
		exists, err := server.isPaymentValid(token, req.PaymentID)
		if err != nil {
			util.LOGGER.Error("POST /api/payments: failed to check if payment record exists or not", "error", err)
			ctx.JSON(http.StatusInternalServerError, CreatePaymentError{Message: "Internal server error"})
			return
		}

		if !exists {
			ctx.JSON(http.StatusBadRequest, CreatePaymentError{
				Message: "Invalid payment ID. Payment must exists and its status must be 'failed' if you want to retry",
			})
			return
		}
	}

	// Create payment intent
	intent, err := payment.CreatePaymentIntent(req.Amount, stripe.CurrencyVND, req.PaymentID)
	if err != nil {
		util.LOGGER.Error("POST /api/payments: failed to create payment intent in Stripe", "error", err)

		// Update payment status to failed
		_, err = server.updatePayment(token, req.PaymentID, map[string]any{"status": "failed"})
		if err != nil {
			util.LOGGER.Error("POST /api/payments: failed to update payment status in Directus", "error", err)
		}

		util.LOGGER.Info("POST /api/payments: failed to create payment intent, success update database status")

		ctx.JSON(http.StatusInternalServerError, CreatePaymentError{
			Message:   "Internal server error! Please use the payment ID provided and retry again",
			PaymentID: req.PaymentID,
		})
		return
	}

	// Update payment transaction_id to payment_intent_id and status to pending
	paymentInfo, err := server.updatePayment(token, req.PaymentID, map[string]any{"transaction_id": intent.ID, "status": "pending"})
	if err != nil {
		util.LOGGER.Error("POST /api/payments: failed to update payment with payment_intent_id and status", "error", err)

		// Clean up, cancel payment intent
		if err = payment.CancelPaymentIntent(intent.ID); err != nil {
			util.LOGGER.Warn("POST /api/payments: failed to cancel payment intent after database update failed", "error", err)
			// This clean up step is just for cleaner Stripe dashboard.
			// When a new request with the same parameters and idempotency key is sent to Stripe again, it gonna return
			// the previous payment intent, so even if we failed here, nothing would happen
		}

		ctx.JSON(http.StatusInternalServerError, CreatePaymentError{
			Message:   "Internal server error! Please use the payment ID provided and retry again",
			PaymentID: req.PaymentID,
		})
		return
	}

	// Return data back to client
	ctx.JSON(http.StatusOK, CreatePaymentResponse{
		Payment:        *paymentInfo,
		ClientSecret:   intent.ClientSecret,
		PublishableKey: server.config.StripePublishableKey,
	})
}

// CreatePaymentMethod godoc
// @Summary      Create payment method
// @Description  Create payment method for confirm payment. This API is solely for internal testing, not to be consumed by any client
// @Tags         Payments
// @Accept       json
// @Produce      json
// @Success      200  {object}  SuccessMessage   "Payment method ID of mock visa"
// @Failure      500  {object}  ErrorResponse                    "Internal server error"
// @Router       /api/payments/method [get]
func (server *Server) CreatePaymentMethod(ctx *gin.Context) {
	pm, err := payment.CreatePaymentMethodFromToken("tok_visa")
	if err != nil {
		util.LOGGER.Error("GET /api/payments/method: failed to create mock token payment method", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	ctx.JSON(http.StatusOK, SuccessMessage{"Payment method: " + pm.ID})
}

type ConfirmPaymentRequest struct {
	PaymentIntentID string `json:"payment_intent_id" binding:"required"`
	PaymentMethodID string `json:"payment_method_id" binding:"required"`
}

// ConfirmPayment godoc
// @Summary      Confirm an existing payment
// @Description  Confirms a Stripe payment intent and updates the payment record in the database with the confirmation status.
// @Tags         Payments
// @Accept       json
// @Produce      json
// @Param        id             path      string                 true   "Payment ID"
// @Param        request        body      ConfirmPaymentRequest   true   "Payment confirmation payload"
// @Success      200  {object}  db.Payment                          "Payment confirmed successfully"
// @Failure      400  {object}  ErrorResponse                    "Invalid request body"
// @Failure      401  {object}  ErrorResponse                    "Unauthorized access"
// @Failure      500  {object}  ErrorResponse                    "Internal server error or failed to confirm payment in Stripe/Directus"
// @Security BearerAuth
// @Router       /api/payments/{id}/confirm [post]
func (server *Server) ConfirmPayment(ctx *gin.Context) {
	// Get access token
	token := server.GetToken(ctx)
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Unauthorized access"})
		return
	}

	// Get request body
	var req ConfirmPaymentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Error("POST /api/payments/:id/confirm: failed to parse request body", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// Check if payment ID exists and payment status must be pending before processing
	paymentID := ctx.Param("id")
	url := fmt.Sprintf("%s/items/payments/%s?fields=id,status", server.config.DirectusAddr, paymentID)
	var paymentInfo db.Payment
	statusCode, err := db.MakeRequest("GET", url, nil, token, &paymentInfo)
	if err != nil {
		util.LOGGER.Error("POST /api/payments/:id/confirm: failed to check if payment ID exists and valid", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Check if payment exists
	if statusCode == http.StatusNotFound {
		util.LOGGER.Warn("POST /api/payments/:id/confirm: payment ID not exists")
		ctx.JSON(http.StatusNotFound, ErrorResponse{"Invalid payment ID, payment ID not exists"})
		return
	}

	// Check payment status: must be in pending state
	if paymentInfo.Status == "failed" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Payment status is 'failed', must be in pending state before confirmation"})
		return
	}

	if paymentInfo.Status == "success" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Payment already success"})
		return
	}

	if paymentInfo.Status == "processing" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Payment currently processed"})
		return
	}

	// Update payment status into processing to avoid spamming
	_, err = server.updatePayment(token, paymentID, map[string]any{"status": "processing"})
	if err != nil {
		util.LOGGER.Error("POST /api/payments/:id/confirm: failed to update payment status to processing", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Check if this payment actuall paid or not, since client can actually retry because of other errors
	isSuccess, err := payment.IsPaymentIntentLastCallSuccess(req.PaymentIntentID)
	if err != nil {
		util.LOGGER.Error("POST /api/payments/:id/confirm: failed to check if payment intent is already paid or not", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// If this payment is not paid, then we can try confirming payment intent
	var body = map[string]any{} // Body for payment update
	if !isSuccess {
		intent, err := payment.ConfirmPaymentIntent(req.PaymentIntentID, req.PaymentMethodID)
		if err != nil {
			// If payment failed -> update payment status in database back to pending
			body["status"] = "pending"
			_, err := server.updatePayment(token, paymentID, body)
			if err != nil {
				util.LOGGER.Error(
					"POST /api/payments/:id/confirm: failed to rollback payment status from processing to pending",
					"error", err,
				)
			}

			// Log and return error message back to client
			util.LOGGER.Error("POST /api/payments/:id/confirm: failed to confirm payment in Stripe", "error", err)
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
			return
		}

		// Check payment confirm status. Unlike with create payment intent, here, confirm failed doesn't necessarily equal error
		// for example, payment method incorrect or account does't have enough money can also lead to unsucess confirmation.
		if intent.Status != "succeeded" {
			// If payment failed -> update payment status in database back to pending
			body["status"] = "pending"
			_, err := server.updatePayment(token, paymentID, body)
			if err != nil {
				util.LOGGER.Error(
					"POST /api/payments/:id/confirm: failed to rollback payment status from processing to pending",
					"error", err,
				)
			}

			// Extract meaningful error message
			errorMsg := "Payment confirmation failed"

			if intent.LastPaymentError != nil {
				switch intent.LastPaymentError.Code {
				case stripe.ErrorCodeCardDeclined:
					errorMsg = "Card declined. Please try a different payment method"
				case stripe.ErrorCodeInsufficientFunds:
					errorMsg = "Insufficient funds. Please use a different card"
				case stripe.ErrorCodeExpiredCard:
					errorMsg = "Card expired. Please use a different card"
				case stripe.ErrorCodeIncorrectCVC:
					errorMsg = "Incorrect CVC. Please check your card details"
				case stripe.ErrorCodeProcessingError:
					errorMsg = "Payment processing error. Please try again"
				default:
					errorMsg = fmt.Sprintf("Payment failed: %s", intent.LastPaymentError.Msg)
				}
			}

			util.LOGGER.Warn("POST /api/payments/:id/confirm: payment not succeeded",
				"status", intent.Status,
				"error_code", intent.LastPaymentError.Code,
				"error_message", intent.LastPaymentError.Msg,
			)

			ctx.JSON(http.StatusPaymentRequired, ErrorResponse{errorMsg})
			return
		}

		// If confirmation success, add the payment method into body payload
		body["payment_method"] = intent.PaymentMethod.Type
		body["status"] = "success"
	}

	// Update payment with Stripe's status returned
	fields := []string{
		"id", "date_created", "amount", "payment_gateway", "payment_method", "status", "booking_id.booking_items.id",
	}
	url = fmt.Sprintf("%s/items/payments/%s?fields=%s", server.config.DirectusAddr, paymentID, strings.Join(fields, ","))
	status, err := db.MakeRequest("PATCH", url, body, token, &paymentInfo)
	if err != nil {
		util.LOGGER.Error("POST /api/payments/:id/confirm: failed to update payment record in database", "status", status, "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{
			"confirmation success, but we hit an issue with database. Please retry again",
		})
		return
	}

	// Check if Booking of payment info is nil, just in case, to avoid nil pointer dereference
	if paymentInfo.Booking == nil {
		util.LOGGER.Error("POST /api/payments/:id/confirm: booking is nil")
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	util.LOGGER.Info("POST /api/payments/:id/confirm: start publishing QRs", "items", len(paymentInfo.Booking.BookingItems))

	// Build the booking_item_id slice for task distributing
	ids := make([]string, len(paymentInfo.Booking.BookingItems))
	for i, item := range paymentInfo.Booking.BookingItems {
		ids[i] = item.ID
	}

	// Distibute background task: publishing QR tickets
	err = server.distributor.DistributeTask(
		ctx,
		worker.PublishQRTicket,
		worker.PublishQRTicketPayload{
			BookingItemIDs: ids,
			CheckInURL:     server.config.CheckinURL,
		},
		asynq.Queue(worker.HIGH_IMPACT),
		asynq.MaxRetry(5),
	)

	if err != nil {
		util.LOGGER.Error("POST /api/payment/:id/confirm: failed to distribute task", "task", worker.PublishQRTicket, "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Payment confirmation success, but failed to publish QR"})
		return
	}

	ctx.JSON(http.StatusOK, paymentInfo)
}

// RetryQRPublishing godoc
// @Summary      Retry QR ticket publishing for a completed payment
// @Description  Re-publishes QR codes for all booking items associated with a successful payment.
// @Description  Used when QR generation failed after payment confirmation.
// @Tags         Payment
// @Accept       json
// @Produce      json
// @Param        id path string true "Payment ID"
// @Success      200  {object}  SuccessMessage  "QR publishing retried successfully"
// @Failure      400  {object}  ErrorResponse   "Invalid payment status or bad request"
// @Failure      401  {object}  ErrorResponse   "Unauthorized access"
// @Failure      404  {object}  ErrorResponse   "Payment ID not found"
// @Failure      500  {object}  ErrorResponse   "Internal server or task distribution error"
// @Security BearerAuth
// @Router       /api/payments/{id}/retry-qr-publishing [post]
func (server *Server) RetryQRPublishing(ctx *gin.Context) {
	// Get access token
	token := server.GetToken(ctx)
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Unauthorized access"})
		return
	}

	// Get request body
	var req ConfirmPaymentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Error("POST /api/payments/:id/confirm: failed to parse request body", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// Check if payment ID exists and payment status must be success before processing
	paymentID := ctx.Param("id")
	url := fmt.Sprintf("%s/items/payments/%s?fields=id,status", server.config.DirectusAddr, paymentID)
	var paymentInfo db.Payment
	statusCode, err := db.MakeRequest("GET", url, nil, token, &paymentInfo)
	if err != nil {
		util.LOGGER.Error("POST /api/payments/:id/confirm: failed to check if payment ID exists and valid", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Check if payment exists
	if statusCode == http.StatusNotFound {
		util.LOGGER.Warn("POST /api/payments/:id/confirm: payment ID not exists")
		ctx.JSON(http.StatusNotFound, ErrorResponse{"Invalid payment ID, payment ID not exists"})
		return
	}

	// Check payment status: must be in success state.
	// Since if QR publishing failed, then all other operation must succeed before, so its status must be success
	if paymentInfo.Status != "success" {
		util.LOGGER.Warn("POST /api/payments/:id/retry-qr-publishing: payment status not success", "status", paymentInfo.Status)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid payment status, its must be success for QR publishing"})
		return
	}

	// Build the booking_item_id slice for task distributing
	ids := make([]string, len(paymentInfo.Booking.BookingItems))
	for i, item := range paymentInfo.Booking.BookingItems {
		ids[i] = item.ID
	}

	// Distibute background task: publishing QR tickets
	err = server.distributor.DistributeTask(
		ctx,
		worker.PublishQRTicket,
		worker.PublishQRTicketPayload{
			BookingItemIDs: ids,
			CheckInURL:     server.config.CheckinURL,
		},
		asynq.Queue(worker.HIGH_IMPACT),
		asynq.MaxRetry(5),
	)

	if err != nil {
		util.LOGGER.Error("POST /api/payment/:id/confirm: failed to distribute task", "task", worker.PublishQRTicket, "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Payment confirmation success, but failed to publish QR"})
		return
	}

	ctx.JSON(http.StatusOK, SuccessMessage{"QR publishing!"})
}

// Refund godoc
// @Summary      Refund a successful payment
// @Description  Initiates a Stripe refund for a completed payment and records it in Directus.
// @Description  Supports both user-requested refunds (partial refund if outside the allowed time window)
// @Description  and automatic refunds (full refund, e.g., event cancellation).
// @Tags         Payments
// @Accept       json
// @Produce      json
// @Param        id                path   string  true   "Payment ID"
// @Param        is_user_requested query  bool    false  "Whether the refund is user-requested (may affect refund amount)"
// @Success      200  {string}  SuccessMessage  "Refund processed successfully"
// @Failure      400  {object}  ErrorResponse  "Invalid payment status or parameters"
// @Failure      401  {object}  ErrorResponse  "Unauthorized access"
// @Failure      404  {object}  ErrorResponse  "Payment not found"
// @Failure      500  {object}  ErrorResponse  "Stripe or Directus internal error"
// @Security BearerAuth
// @Router       /api/payments/{id}/refund [post]
func (server *Server) Refund(ctx *gin.Context) {
	// Get access token
	token := server.GetToken(ctx)
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Unauthorized access"})
		return
	}

	// Get payment ID from path parameter
	paymentID := ctx.Param("id")

	// Try get payment info
	var paymentInfo db.Payment
	fields := []string{
		"id", "date_created", "transaction_id", "amount", "status",
	}
	url := fmt.Sprintf("%s/items/payments/%s?fields=%s", server.config.DirectusAddr, paymentID, strings.Join(fields, ","))
	status, err := db.MakeRequest("GET", url, nil, token, &paymentInfo)
	if err != nil {
		util.LOGGER.Error("POST /api/payments/:id/refund: failed to get payment info", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	if status == http.StatusNotFound {
		ctx.JSON(status, ErrorResponse{"Payment ID not exists"})
		return
	}

	if paymentInfo.Status != "success" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"A payment must success first before refund: " + paymentInfo.Status})
		return
	}

	// Get request parameter: is_user_requested
	// If this parameter is set to true, we'll check if they can get a 100% refund
	// If not set or set to false, then this would be a 'passive' refund, for example, event canceled,
	// which always leads to a full refund
	var (
		isUserRequested = ctx.Query("is_user_requested")
		amount          = paymentInfo.Amount
		reason          = "" // This reason is used for database, not Stripe
		refundStatus    = "" // This status will be standardize based on Stripe result, so that we can integrate with other gateway
	)
	if isUserRequested = strings.TrimSpace(strings.ToLower(isUserRequested)); isUserRequested == "true" {
		// date_created is an auto-generated and auto-managed field for Directus when creating collection
		// so the chance it's being nil is almost none if Directus setup is correctly
		reason = "user-request"
		if time.Time(*paymentInfo.DateCreated).Add(time.Hour * time.Duration(server.config.MaxFullRefundHours)).Before(time.Now()) {
			amount /= 2
		}
	} else {
		reason = "auto-refund"
	}

	// Refund. Since Stripe only allow for 3 reasons that was defined in their API, we're gonna use requested by customer
	intent, err := payment.CreateRefund(paymentInfo.TransactionID, payment.RequestedByCustomer, int64(amount))
	if err != nil {
		util.LOGGER.Error("POST /api/payments/:id/refund: failed to create refund", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	if intent.Status == "succeeded" {
		refundStatus = "success"
	} else {
		refundStatus = "failed"
	}

	// Create refund record
	url = fmt.Sprintf("%s/items/refunds", server.config.DirectusAddr)
	status, err = db.MakeRequest("POST", url, map[string]any{
		"amount":     amount,
		"reason":     reason,
		"status":     refundStatus,
		"payment_id": paymentInfo.ID,
	}, token, nil)

	if err != nil {
		util.LOGGER.Error("POST /api/payments/:id/refund: failed to create refund record in database", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, SuccessMessage{"Refund success"})
}
