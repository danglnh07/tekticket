package api

import (
	"fmt"
	"net/http"
	"strings"
	"tekticket/db"
	"tekticket/service/payment"
	"tekticket/service/worker"
	"tekticket/util"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/stripe/stripe-go/v82"
)

type CreatePaymentRequest struct {
	BookingID string `json:"booking_id" binding:"required"`
	Amount    uint   `json:"amount" binding:"required"`
}

type CreatePaymentResponse struct {
	Payment        db.Payment `json:"payment"`
	ClientSecret   string     `json:"client_secret"`
	PublishableKey string     `json:"publishable_key"`
}

// CreatePayment godoc
// @Summary      Create a new payment
// @Description  Creates a Stripe payment intent and records the payment information in the database for a booking.
// @Tags         Payments
// @Accept       json
// @Produce      json
// @Param        request        body      CreatePaymentRequest  true   "Payment creation payload"
// @Success      200  {object}  CreatePaymentResponse           "Payment created successfully"
// @Failure      400  {object}  ErrorResponse                   "Invalid request body"
// @Failure      401  {object}  ErrorResponse                   "Unauthorized access"
// @Failure      500  {object}  ErrorResponse                   "Internal server error or failed to communicate with Stripe/Directus"
// @Security BearerAuth
// @Router       /api/payments [post]
func (server *Server) CreatePayment(ctx *gin.Context) {
	// Get access token
	token := server.GetToken(ctx)
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{"Unauthorized access"})
		return
	}

	// Get request body
	var req CreatePaymentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Error("POST /api/payments: failed to parse request body", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// Create payment intent in Stripe
	intent, err := payment.CreatePaymentIntent(int64(req.Amount), stripe.CurrencyVND)
	if err != nil {
		util.LOGGER.Error("POST /api/payments: failed to create Stripe payment intent", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Create payment record in database
	fields := []string{
		"id", "transaction_id", "amount", "payment_gateway", "status", "booking_id.id", "date_created",
	}
	url := fmt.Sprintf("%s/items/payments?fields=%s", server.config.DirectusAddr, strings.Join(fields, ","))
	var payment db.Payment
	status, err := db.MakeRequest("POST", url, map[string]any{
		"transaction_id": intent.ID,
		"amount":         req.Amount,
		"booking_id":     req.BookingID,
		"status":         "pending",
	}, token, &payment)

	if err != nil {
		util.LOGGER.Error("POST /api/payments: failed to record payment into database", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, CreatePaymentResponse{
		Payment:        payment,
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

	// Confirm payment
	intent, err := payment.ConfirmPaymentIntent(req.PaymentIntentID, req.PaymentMethodID)
	if err != nil {
		util.LOGGER.Error("POST /api/payments/:id/confirm: failed to confirm payment in Stripe", "error", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Log the payment confirm status
	util.LOGGER.Info("POST /api/payments/:id/confirm: confirm status", "status", intent.Status)

	// Update payment with Stripe's status returned
	fields := []string{
		"id", "amount", "payment_gateway", "payment_method", "status", "booking.booking_items.id",
	}
	url := fmt.Sprintf("%s/items/payments/%s?fields=%s", server.config.DirectusAddr, ctx.Param("id"), strings.Join(fields, ","))
	var paymentInfo db.Payment
	status, err := db.MakeRequest(
		"PATCH",
		url,
		map[string]any{
			"payment_method": intent.PaymentMethod.Type,
			"status":         intent.Status,
		},
		token,
		&paymentInfo,
	)
	if err != nil {
		util.LOGGER.Error("POST /api/payments/:id/confirm: failed to update payment record in database", "error", err)
		ctx.JSON(status, ErrorResponse{err.Error()})
		return
	}

	// Start background task: publish QR tickets
	if paymentInfo.Booking != nil {
		for _, item := range paymentInfo.Booking.BookingItems {
			err := server.distributor.DistributeTask(ctx, worker.PublishQRTicket, worker.PublishQRTicketPayload{
				BookingItemID: item.ID,
				CheckInURL:    server.config.CheckinURL,
			}, asynq.Queue(worker.HIGH_IMPACT), asynq.MaxRetry(5))

			if err != nil {
				util.LOGGER.Error(
					"POST /api/payment/:id/confirm: failed to distribute task",
					"task", worker.PublishQRTicket,
					"booking_item_id", item.ID,
					"error", err,
				)

				// If failed to publish ticket, we'll refund money back to user
				refund, err := payment.CreateRefund(intent.ID, payment.RequestedByCustomer, intent.Amount)
				if err != nil {
					util.LOGGER.Error(
						"POST /api/payments/:id/confirm: failed to refund to customer after failing publishing tickets",
						"error", err,
					)
					ctx.JSON(http.StatusInternalServerError, ErrorResponse{
						"Payment success, but failed to publish tickets! We try to refund but failed again. Please contact the admin",
					})
					return
				}

				// Check if refund success
				if refund.Status != "succeeded" {
					util.LOGGER.Error("POST /api/payments/:id/confirm: refund failed", "status", refund.Status)
					ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Refund failed: " + string(refund.Status)})
					return
				}

				ctx.JSON(http.StatusInternalServerError, ErrorResponse{
					"Payment success, but failed to publish ticket. We've refuned your previous payment",
				})
				return
			}
		}
	}

	ctx.JSON(http.StatusOK, paymentInfo)
}
