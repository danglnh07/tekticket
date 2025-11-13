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

// Helper method: ensure that a payment record always exists in database for create payment to work
func (server *Server) ensurePaymentRecordExists(token, paymentID, bookingID string, amount int64) (*db.Payment, int, error) {
	// If payment ID is provided, then we check if the this payment ID is valid (exists in database with status 'failed' for retry)
	if paymentID = strings.TrimSpace(paymentID); paymentID != "" {
		url := fmt.Sprintf("%s/items/payments/%s?fields=id,status", server.config.DirectusAddr, paymentID)
		var paymentInfo db.Payment

		status, err := db.MakeRequest("GET", url, nil, token, &paymentInfo)
		if err != nil {
			return nil, status, err
		}

		if paymentInfo.Status != "failed" {
			return nil, http.StatusOK, nil
		}

		return &paymentInfo, http.StatusOK, nil
	}

	// If payment ID is not provided, then we create a new payment record
	url := fmt.Sprintf("%s/items/payments?fields=id", server.config.DirectusAddr)
	body := map[string]any{
		"amount":     amount,
		"booking_id": bookingID,
		"status":     "pending",
	}

	var paymentInfo db.Payment
	status, err := db.MakeRequest("POST", url, body, token, &paymentInfo)

	if err != nil {
		return nil, status, err
	}

	return &paymentInfo, http.StatusOK, nil
}

type CreatePaymentRequest struct {
	BookingID string `json:"booking_id" binding:"required"`
	Amount    int64  `json:"amount" binding:"required"`
	PaymentID string `json:"payment_id"` // Used for retry
}

type CreatePaymentResponse struct {
	PaymentID      string `json:"payment_id"`      // Database ID
	TransactionID  string `json:"transaction_id"`  // Stripe payment_intent_id
	PublishableKey string `json:"publishable_key"` // Stripe publishable key
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

	// Get request body
	var req CreatePaymentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Warn("POST /api/payments: failed to bind request body", "error", err)
		ctx.JSON(http.StatusBadRequest, CreatePaymentError{Message: "Invalid request body"})
		return
	}

	// Check if amount is valid. For VND currency, Stripe only accept amount in range [100, 99_999_999]
	if req.Amount < 100 || req.Amount > 99_999_999 {
		ctx.JSON(http.StatusBadRequest, CreatePaymentError{Message: "Payment amount must be in range [100, 99.999.999] for VND"})
		return
	}

	// Ensure that a paymentID always exists for Stripe create payment intent, since we use paymentID as the idempotency key
	paymentInfo, status, err := server.ensurePaymentRecordExists(token, req.PaymentID, req.BookingID, req.Amount)
	if err != nil {
		util.LOGGER.Error("POST /api/payments: failed to ensure that payment record exists", "status", status, "error", err)
		server.DirectusError(ctx, err)
		return
	}

	if paymentInfo == nil {
		// If error is nil but paymentInfo is nil -> req.PayementID is provided and correct,
		// but the payment status is not 'failed' -> cannot retry
		util.LOGGER.Warn("POST /api/payments: payment is nil, skip this request")
		ctx.JSON(http.StatusBadRequest, CreatePaymentError{Message: "Invalid payment ID for retry"})
		return
	}

	// Create payment intent
	intent, err := payment.CreatePaymentIntent(req.Amount, stripe.CurrencyVND, paymentInfo.ID)
	if err != nil {
		util.LOGGER.Error("POST /api/payments: failed to create payment intent in Stripe", "error", err)

		// We create a background task for retry, in case database is down and the update didn't work somehow
		payload := worker.UpdatePaymentRecordPayload{
			URL:     fmt.Sprintf("%s/items/payments/%s", server.config.DirectusAddr, paymentInfo.ID),
			Body:    map[string]any{"status": "failed"},
			Token:   token,
			Caller:  "POST /api/payments",
			Context: "rollback payment status to 'failed' after creat payment intent in Stripe failed",
		}

		err = server.distributor.DistributeTask(
			ctx,
			worker.UpdatePaymentRecord,
			payload,
			asynq.Queue(worker.HIGH_IMPACT),
			asynq.MaxRetry(5),
		)

		if err != nil {
			// If even task distributing failed, the only thing we can do is log and manually fix the problem :v
			util.LOGGER.Error(
				"POST /api/payments: failed to distribute background task",
				"task_issued_reason", "update payment after create payment intent failed",
				"error", err,
			)
		}

		ctx.JSON(http.StatusInternalServerError, CreatePaymentError{
			Message:   "Internal server error! Please use the payment ID provided and retry again",
			PaymentID: req.PaymentID,
		})
		return
	}

	// Update payment transaction_id to payment_intent_id and status to pending
	payload := worker.UpdatePaymentRecordPayload{
		URL:     fmt.Sprintf("%s/items/payments/%s", server.config.DirectusAddr, paymentInfo.ID),
		Body:    map[string]any{"transaction_id": intent.ID, "status": "pending"},
		Token:   token,
		Caller:  "POST /api/payyments",
		Context: "update payment with transaction_id and status = pending after create payment intent success",
	}

	err = server.distributor.DistributeTask(
		ctx,
		worker.UpdatePaymentRecord,
		payload,
		asynq.Queue(worker.HIGH_IMPACT),
		asynq.MaxRetry(5),
	)

	if err != nil {
		util.LOGGER.Error(
			"POST /api/payments: failed to distribute background tasks",
			"task_issued_reason", "update payment after create payment intent success",
			"error", err,
		)
	}

	// Return data back to client
	ctx.JSON(http.StatusOK, CreatePaymentResponse{
		PaymentID:      paymentInfo.ID,
		TransactionID:  intent.ID,
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
// @Security BearerAuth
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

// Helper method: extract reason for payment confirmation or refund failed
func (server *Server) extractFailedPaymentReason(intent *stripe.PaymentIntent) (int, string) {
	// If payment failed but last payment error is nil (which somehow contradict, we call it some unexpected error)
	if intent.LastPaymentError == nil {
		return http.StatusInternalServerError, "unexpected error"
	}

	var (
		reason = ""
		status = intent.LastPaymentError.HTTPStatusCode
	)

	// In Stripe viewpoint, it can be internal server error, but from our view point, it's not, so using failed dependency here
	// make more sense
	if status == http.StatusInternalServerError {
		status = http.StatusFailedDependency
	}

	// Using some common error, craft a user-friendly reason
	switch intent.LastPaymentError.Code {
	case stripe.ErrorCodeCardDeclined:
		reason = "Card declined. Please try a different payment method"
	case stripe.ErrorCodeInsufficientFunds:
		reason = "Insufficient funds. Please use a different card"
	case stripe.ErrorCodeExpiredCard:
		reason = "Card expired. Please use a different card"
	case stripe.ErrorCodeIncorrectCVC:
		reason = "Incorrect CVC. Please check your card details"
	case stripe.ErrorCodeProcessingError:
		reason = "Payment processing error. Please try again"
	default:
		reason = fmt.Sprintf("Payment failed: %s", intent.LastPaymentError.Msg)
	}

	return status, reason
}

type ConfirmPaymentRequest struct {
	PaymentIntentID string `json:"payment_intent_id" binding:"required"`
	PaymentMethodID string `json:"payment_method_id" binding:"required"`
}

type ConfirmPaymentResponse struct {
	Message string `json:"message"`
	Amount  int64  `json:"amount"`
	Date    string `json:"date"`
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

	// Get request body
	var req ConfirmPaymentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Error("POST /api/payments/:id/confirm: failed to bind request body", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// Check if payment ID exists and payment status must be pending before processing
	paymentID := ctx.Param("id")
	url := fmt.Sprintf("%s/items/payments/%s?fields=id,status", server.config.DirectusAddr, paymentID)
	var paymentInfo db.Payment
	status, err := db.MakeRequest("GET", url, nil, token, &paymentInfo)
	if err != nil {
		util.LOGGER.Error(
			"POST /api/payments/:id/confirm: failed to check if payment exists",
			"id", paymentID,
			"status", status,
			"error", err,
		)
		server.DirectusError(ctx, err)
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

	// Check if this payment actuall paid or not
	intent, err := payment.GetPaymentIntent(req.PaymentIntentID)
	if err != nil {
		util.LOGGER.Error("POST /api/payments/:id/confirm: failed to get payment intent from Stripe", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	if intent.Status != stripe.PaymentIntentStatusRequiresPaymentMethod {
		util.LOGGER.Warn("POST /api/payments/:id/confirm: payment intent status invalid, skip this request", "status", intent.Status)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid payment intent ID"})
		return
	}

	// Update payment status into processing to avoid spamming. Since this is the first operation, no need to retry
	url = fmt.Sprintf("%s/items/payments/%s", server.config.DirectusAddr, paymentID)
	status, err = db.MakeRequest("PATCH", url, map[string]any{"status": "processing"}, token, nil)
	if err != nil {
		util.LOGGER.Error("POST /api/payments/:id/confirm: failed to update payment status to processing", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Confirm payment
	confirmIntent, err := payment.ConfirmPaymentIntent(req.PaymentIntentID, req.PaymentMethodID)
	if err != nil {
		util.LOGGER.Error("POST /api/payments/:id/confirm: failed to confirm payment intent", "error", err)

		// Rollback: update payment status from 'processing' to 'pending'
		payload := worker.UpdatePaymentRecordPayload{
			URL:     fmt.Sprintf("%s/items/payments/%s", server.config.DirectusAddr, paymentID),
			Body:    map[string]any{"status": "pending"},
			Token:   token,
			Caller:  "POST /api/payments/:id/confirm",
			Context: "rollback after payment confirmation error",
		}

		err = server.distributor.DistributeTask(
			ctx,
			worker.UpdatePaymentRecord,
			payload,
			asynq.Queue(worker.HIGH_IMPACT),
			asynq.MaxRetry(5),
		)

		if err != nil {
			util.LOGGER.Error(
				"POST /api/payments/:id/confirm: failed to distribute background task",
				"task_issued_reason", "rollback payment status after payment confirmation error",
				"error", err,
			)
		}

		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Check if confirmation actually success. A failure can still occur, even if no error is return
	if confirmIntent.Status != "succeeded" {
		util.LOGGER.Warn("POST /api/payments/:id/confirm: payment confirmation failed", "status", confirmIntent.Status)

		// Try getting the reason why payment confirmation failed
		status, reason := server.extractFailedPaymentReason(confirmIntent)

		// Rollback: update payment status from 'processing' to 'pending'
		payload := worker.UpdatePaymentRecordPayload{
			URL:     fmt.Sprintf("%s/items/payments/%s", server.config.DirectusAddr, paymentID),
			Body:    map[string]any{"status": "pending"},
			Token:   token,
			Caller:  "POST /api/payments/:id/confirm",
			Context: "rollback after payment confirmation failure",
		}

		err = server.distributor.DistributeTask(
			ctx,
			worker.UpdatePaymentRecord,
			payload,
			asynq.Queue(worker.HIGH_IMPACT),
			asynq.MaxRetry(5),
		)

		if err != nil {
			util.LOGGER.Error(
				"POST /api/payments/:id/confirm: failed to distribute background task",
				"task_issued_reason", "rollback payment status after payment confirmation failure",
				"error", err,
			)
		}

		ctx.JSON(status, ErrorResponse{reason})
		return
	}

	// Update payment with payment method type and status = success
	util.LOGGER.Info("POST /api/payments/:id/confirm", "payment_method", confirmIntent.PaymentMethod)
	payload := worker.UpdatePaymentRecordPayload{
		URL:     fmt.Sprintf("%s/items/payments/%s", server.config.DirectusAddr, paymentID),
		Body:    map[string]any{"payment_method": "visa", "status": "success"},
		Token:   token,
		Caller:  "POST /api/payments/:id/confirm",
		Context: "update payment with payment_method and status after payment confirmation success",
	}

	err = server.distributor.DistributeTask(
		ctx,
		worker.UpdatePaymentRecord,
		payload,
		asynq.Queue(worker.HIGH_IMPACT),
		asynq.MaxRetry(5),
	)

	if err != nil {
		util.LOGGER.Error(
			"POST /api/payments/:id/confirm: failed to distribute background task",
			"task_issued_reason", "update payment with payment_method and status after payment confirmation success",
			"error", err,
		)
	}

	ctx.JSON(http.StatusOK, ConfirmPaymentResponse{
		Message: "Payment complete",
		Amount:  confirmIntent.Amount,
		Date:    time.Now().String(),
	})
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

	// Get payment ID from path parameter
	paymentID := ctx.Param("id")

	// Try get payment info
	var paymentInfo db.Payment
	fields := []string{"id", "date_created", "transaction_id", "amount", "status"}
	url := fmt.Sprintf("%s/items/payments/%s?fields=%s", server.config.DirectusAddr, paymentID, strings.Join(fields, ","))
	status, err := db.MakeRequest("GET", url, nil, token, &paymentInfo)
	if err != nil {
		util.LOGGER.Error("POST /api/payments/:id/refund: failed to get payment info", "error", err)
		server.DirectusError(ctx, err)
		return
	}

	// Check if payment status is success or not
	if paymentInfo.Status != "success" {
		util.LOGGER.Warn("POST /api/payments/:id/refund: payment status not success, skip this request", "status", paymentInfo.Status)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"A payment must success first before refund"})
		return
	}

	// Check if this payment can get a full refund, or just a partial refund based on the payment created time
	amount := paymentInfo.Amount
	if time.Time(*paymentInfo.DateCreated).Add(time.Hour * time.Duration(server.config.MaxFullRefundHours)).Before(time.Now()) {
		amount /= 2
	}

	// Create the refund record with status pending
	url = fmt.Sprintf("%s/items/refunds?fields=id", server.config.DirectusAddr)
	var refundRecord db.Refund
	body := map[string]any{
		"amount":     amount,
		"status":     "pending",
		"payment_id": paymentInfo.ID,
		"reason":     "user-canceled",
	}
	status, err = db.MakeRequest("POST", url, body, token, &refundRecord)
	if err != nil {
		util.LOGGER.Error(
			"POST /api/payments/:id/refund: failed to create refund record with status pending",
			"status", status,
			"error", err,
		)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Refund. Since Stripe only allow for 3 reasons that was defined in their API, we're gonna use requested by customer
	refund, err := payment.CreateRefund(paymentInfo.TransactionID, payment.RequestedByCustomer, int64(amount))
	if err != nil {
		util.LOGGER.Error("POST /api/payments/:id/refund: failed to request refund in Stripe", "error", err)

		// Rollback, update refund status back to failed
		payload := worker.UpdatePaymentRecordPayload{
			URL:     fmt.Sprintf("%s/items/refunds/%s", server.config.DirectusAddr, refundRecord.ID),
			Body:    map[string]any{"status": "failed"},
			Token:   token,
			Caller:  "POST /api/payments/:id/refund",
			Context: "rollback refund with status failed after failling refund on Stripe",
		}

		err = server.distributor.DistributeTask(
			ctx,
			worker.UpdatePaymentRecord,
			payload,
			asynq.Queue(worker.HIGH_IMPACT),
			asynq.MaxRetry(5),
		)

		if err != nil {
			util.LOGGER.Error(
				"POST /api/payments/:id/refund: failed to distribute background task",
				"task_issued_reason", "rollback after refund failed",
				"error", err,
			)
		}

		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Check if the refund success or not. Just like with confirm, a refund failure does not mean an error.
	if refund.Status != stripe.RefundStatusSucceeded {
		// Unlike with intent, Stripe refund object only has a small reason for failured, with no HTTP code return
		// Most of the refund failure reason seems like it client side more than server side, so we'll return 400 here
		util.LOGGER.Warn(
			"POST /api/payments/:id/refund: refund failed",
			"status", string(refund.Status),
			"reason", string(refund.FailureReason),
		)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Refund failed: " + string(refund.FailureReason)})
		return
	}

	// Update refund record
	payload := worker.UpdatePaymentRecordPayload{
		URL:     fmt.Sprintf("%s/items/refunds/%s", server.config.DirectusAddr, refundRecord.ID),
		Body:    map[string]any{"status": "success"},
		Token:   token,
		Caller:  "POST /api/payments/:id/refund",
		Context: "update refund with status success after succeeding refund on Stripe",
	}

	err = server.distributor.DistributeTask(
		ctx,
		worker.UpdatePaymentRecord,
		payload,
		asynq.Queue(worker.HIGH_IMPACT),
		asynq.MaxRetry(5),
	)

	if err != nil {
		util.LOGGER.Error(
			"POST /api/payments/:id/refund: failed to distribute background task",
			"task_issued_reason", "update after refund succeeded",
			"error", err,
		)
	}

	ctx.JSON(http.StatusOK, SuccessMessage{"Refund success"})
}
