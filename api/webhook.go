package api

import (
	"fmt"
	"net/http"
	"strings"
	"tekticket/db"
	"tekticket/service/bot"
	"tekticket/service/payment"
	"tekticket/service/worker"
	"tekticket/util"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/stripe/stripe-go/v82"
)

func (server *Server) isChatRegistered(ctx *gin.Context, chatID int) (bool, int, error) {
	// Check cache
	_, err := server.queries.GetCache(ctx, fmt.Sprintf("%d", chatID))
	if err == nil {
		return true, http.StatusOK, nil
	}

	// Check database
	url := fmt.Sprintf("%s/items/user_telegrams?fields=id&filter[telegram_chat_id][_eq]=%d", server.config.DirectusAddr, chatID)
	var userTelegrams []db.UserTelegram
	status, err := db.MakeRequest("GET", url, nil, server.config.DirectusStaticToken, &userTelegrams)
	return len(userTelegrams) != 0, status, err
}

func (server *Server) isUserExists(email, role string) (string, int, error) {
	url := fmt.Sprintf(
		"%s/users?fields=id&filter[email][_eq]=%s&filter[role][name][_icontains]=%s",
		server.config.DirectusAddr,
		email,
		role,
	)
	var users []db.User
	status, err := db.MakeRequest("GET", url, nil, server.config.DirectusStaticToken, &users)
	if err != nil {
		return "", status, err
	}

	if len(users) == 0 {
		return "", http.StatusNotFound, nil
	}

	return users[0].ID, http.StatusOK, nil
}

func (server *Server) sendTelegramMessage(chatID int, message string, isWarning bool) {
	if isWarning {
		message = util.FormatWarningHTML(message)
	}

	if err := server.bot.SendMessage(chatID, message); err != nil {
		util.LOGGER.Error("POST /api/webhook/telegram: failed to send message", "error", err)
	}
}

// Telegram webhook that will listen to any message that user send to the bot.
func (server *Server) TelegramWebhook(ctx *gin.Context) {
	// Get the update request
	var req bot.TelegramUpdate
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Warn("POST /api/webhook/telegram: failed to parse incoming update body", "error", err)
		return
	}

	chatID := req.Message.Chat.ID
	message := strings.TrimSpace(req.Message.Text)
	util.LOGGER.Info("Receive telegram message", "chat_id", chatID, "message", message)

	// Send chat action indicate we are processing
	if err := server.bot.SendChatAction(chatID, bot.CHAT_ACTION); err != nil {
		util.LOGGER.Error("POST /api/webhook/telegram: failed to send the initial chat action", "error", err)
	}

	// Check if this is a Telegram chatbot command or just a simple message
	segments := strings.Split(message, " ")
	if len(segments) == 0 {
		util.LOGGER.Warn("POST /api/webhook/telegram: user sent an empty message, ignore this message")
		return
	}

	command := segments[0]
	arguments := segments[1:]

	// Act based on the command
	switch command {
	case "/register":
		/*
		 * Command: /start <YOUR_EMAIL> <YOUR_ROLE>
		 * If role not provided, assume it to be customer
		 * Flows:
		 * 1. Check if this chatID has already be register in the user_telegrams collections
		 * 2. If not reistered yet, check if credential provided is valid (email exists in database, role is valid)
		 * 3. If all data is valid, create an instance user_telegram collection
		 */

		// Check if at least email exists in the command arguments
		if len(arguments) == 0 {
			server.sendTelegramMessage(chatID, "You must provide your email for registration!", true)
			return
		}

		// Check if current chat has registered for Telegram service
		isRegistered, status, err := server.isChatRegistered(ctx, chatID)
		if err != nil {
			util.LOGGER.Error(
				"POST /api/webhook/telegram: failed to check if telegram chat has been registered or not",
				"status", status,
				"error", err,
			)
			server.sendTelegramMessage(chatID, "Internal server error! Please try again :(", true)
			return
		}

		if isRegistered {
			server.sendTelegramMessage(chatID, "You have already registered, this you forgot?", false)
			return
		}

		// Get the list of all users with the provided email
		userID, status, err := server.isUserExists(arguments[0], arguments[1])
		if err != nil {
			util.LOGGER.Error("POST /api/webhook/telegram: failed to check if email with role exists", "status", status, "error", err)
			server.sendTelegramMessage(chatID, "Internal server error! Please try again :(", true)
			return
		}

		if userID == "" {
			server.sendTelegramMessage(chatID, "No such user with this email and role", false)
			return
		}

		// If exists, we add new entry to the user_telegram collections
		url := fmt.Sprintf("%s/items/user_telegrams", server.config.DirectusAddr)
		status, err = db.MakeRequest("POST", url, map[string]any{
			"telegram_chat_id": fmt.Sprintf("%d", chatID),
			"user_id":          userID,
		}, server.config.DirectusStaticToken, nil)

		if err != nil {
			util.LOGGER.Error(
				"POST /api/webhook/telegram: failed to create instance in user_telegram collection",
				"status", status,
				"error", err,
			)
			server.sendTelegramMessage(chatID, "Internal server error! Please try again :(", true)
			return
		}

		server.sendTelegramMessage(chatID, "Success, now you can start receiving my notification :)", false)

		// Store the current chatID into cache
		server.queries.SetCache(ctx, fmt.Sprintf("%d", chatID), "", time.Hour) // The value can be whatever, we don't really care
	default:
		// server.sendTelegramMessage(chatID, "This is an echo message hehe: "+message, false)
		server.bot.SendMessage(chatID, "This is an echo message hehe :"+message)
	}
}

type NotificationRequest struct {
	Name         string `json:"name"`          // Event name (in can be the notification category)
	Title        string `json:"title"`         // Notification title
	Body         string `json:"body"`          // Notification body
	Queue        string `json:"queue"`         // The impact of this notification. It can be: low, default or critical
	DestEmail    string `json:"dest_email"`    // The destination email, if allow sending email notification
	DestInApp    string `json:"dest_inapp"`    // The channel to send the in app notification using Pub/Sub model
	DestTelegram int    `json:"dest_telegram"` // The chat ID of telegram, if allow telegram notification
}

// NotificationWebhook godoc
// @Summary      Handle Directus notification webhook
// @Description  Receives webhook payloads from Directus flows and dispatches notifications to various destinations (in-app, Telegram, email) using background workers.
// @Tags         Webhook
// @Accept       json
// @Produce      json
// @Param        request  body  NotificationRequest  true  "Notification webhook payload"
// @Success      200  {object}  SuccessMessage       "Notification dispatched successfully"
// @Failure      400  {object}  ErrorResponse        "Invalid request body or missing required destinations"
// @Failure      500  {object}  ErrorResponse        "Internal server error or failed to distribute background task"
// @Router       /api/webhook/notifications [post]
func (server *Server) NotificationWebhook(ctx *gin.Context) {
	// This webhook is used for Directus's flows to send notification back to the server for processing
	var req NotificationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Error("POST /api/notification/webhook: failed to parse request", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	util.LOGGER.Info("Receive notification",
		"telegram", req.DestTelegram,
		"email", req.DestEmail,
		"title", req.Title,
		"body", req.Body,
		"name", req.Name,
	)

	if req.Queue = strings.ToLower(req.Queue); !worker.IsQueueLevelExists(req.Queue) {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Only accept low, default or critical for queue value"})
	}

	// Send notification to in app channel
	if req.DestInApp != "" {
		err := server.distributor.DistributeTask(
			ctx,
			worker.SendInAppNotification,
			worker.SendNotificationPayload{
				Name:  req.Name,
				Title: req.Title,
				Body:  req.Body,
				Dest: worker.NotificationChannel{
					Email:   req.DestEmail,
					Channel: req.DestInApp,
					ChatID:  req.DestTelegram,
				},
			},
			asynq.MaxRetry(25),
			asynq.Queue(req.Queue),
		)
		if err != nil {
			util.LOGGER.Error(
				"POST /api/webhook/notifications: failed to distribute task",
				"task", worker.SendInAppNotification,
				"error", err,
			)
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
			return
		}
	}

	if req.DestTelegram != 0 {
		err := server.distributor.DistributeTask(
			ctx,
			worker.SendTelegramNotification,
			worker.SendNotificationPayload{
				Name:  req.Name,
				Title: req.Title,
				Body:  req.Body,
				Dest: worker.NotificationChannel{
					Email:   req.DestEmail,
					Channel: req.DestInApp,
					ChatID:  req.DestTelegram,
				},
			},
			asynq.MaxRetry(1),
			asynq.Queue(req.Queue),
		)
		if err != nil {
			util.LOGGER.Error(
				"POST /api/webhook/notifications: failed to distribute task",
				"task", worker.SendTelegramNotification,
				"error", err,
			)
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
			return
		}
	}

	if req.DestEmail != "" {
		err := server.distributor.DistributeTask(
			ctx,
			worker.SendEmailNotification,
			worker.SendNotificationPayload{
				Name:  req.Name,
				Title: req.Title,
				Body:  req.Body,
				Dest: worker.NotificationChannel{
					Email:   req.DestEmail,
					Channel: req.DestInApp,
					ChatID:  req.DestTelegram,
				},
			},
			asynq.MaxRetry(1),
			asynq.Queue(req.Queue),
		)
		if err != nil {
			util.LOGGER.Error(
				"POST /api/webhook/notifications: failed to distribute task",
				"task", worker.SendEmailNotification,
				"error", err,
			)
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
			return
		}
	}
}

// Publish QR webhook
type PublishQRTicketsRequest struct {
	BookingItemIDs []string `json:"booking_item_ids" binding:"required"`
}

func (server *Server) PublishQRTickets(ctx *gin.Context) {
	// Get and validate request body
	var req PublishQRTicketsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Warn("POST /api/tickets/pulish: failed to bind request body", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// Distribute background task: publish QR
	err := server.distributor.DistributeTask(
		ctx,
		worker.PublishQRTicket,
		worker.PublishQRTicketPayload{
			BookingItemIDs: req.BookingItemIDs,
			CheckInURL:     server.config.CheckinURL,
		},
		asynq.Queue(worker.MEDIUM_IMPACT),
		asynq.MaxRetry(5),
	)

	if err != nil {
		util.LOGGER.Error(
			"POST /api/webhook/tickets/publish: failed to distribute background task",
			"task", worker.PublishQRTicket,
			"error", err,
		)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	ctx.JSON(http.StatusOK, SuccessMessage{"Start publishing QR"})
}

// Refund webhook for event-cancelling -> called by Directus
type RefundRequest struct {
	PaymentIntentID string `json:"payment_intent_id"`
	Amount          int64  `json:"amount"`
}

func (server *Server) RefundWebhook(ctx *gin.Context) {
	// Here, what we really do is just call refund in Stripe, all of the database update/rollback should be handled by Directus flow
	var req RefundRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Warn("POST /api/webhook/refund: failed to bind request body", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	refund, err := payment.CreateRefund(req.PaymentIntentID, payment.RequestedByCustomer, req.Amount)
	if err != nil {
		util.LOGGER.Error("POST /api/webhook/refund: failed to create refund", "err", err)
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
		return
	}

	// Check if the refund success or not. Just like with confirm, a refund failure does not mean an error.
	if refund.Status != stripe.RefundStatusSucceeded {
		// Unlike with intent, Stripe refund object only has a small reason for failured, with no HTTP code return
		// Most of the refund failure reason seems like it client side more than server side, so we'll return 400 here
		util.LOGGER.Warn(
			"POST /api/webhook/refund: refund failed",
			"status", string(refund.Status),
			"reason", string(refund.FailureReason),
		)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Refund failed: " + string(refund.FailureReason)})
		return
	}

	ctx.JSON(http.StatusOK, SuccessMessage{"Refund success"})
}

// Webhook godoc
// @Summary      User profile
// @Description  Get user profile
// @Tags         Webhook
// @Accept       json
// @Produce      json
// @Param        request  body  db.Setting true  "Settings"
// @Success      200  {object}  SuccessMessage      "Update configuration success"
// @Failure      400  {object}  ErrorResponse        "Invalid request body"
// @Router       /api/webhook/settings [post]
func (server *Server) SettingWebhook(ctx *gin.Context) {
	// Get request body and validate
	var req db.Setting
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Warn("POST /api/webhook/settings: failed to bind request body", "error", err)
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// Since both Server and RedisTaskProcessor hold the same refernce to all services, update in Server will reflect
	// changes in processor
	errMsgs := []string{}

	// Check if the chance made is the max worker, to scale the worker service
	if req.MaxWorkers != 0 {
		util.LOGGER.Info("POST /api/webhook/settings: detect max_workers changed, start scale up/down server")

		if err := server.processor.Scale(req.MaxWorkers, server.config.RedisAddr); err != nil {
			util.LOGGER.Error("POST /api/webhook/settings: failed to start background server with new workers setup", "error", err)
			errMsgs = append(errMsgs, "failed to scale worker server: %s", err.Error())
		}

		util.LOGGER.Info("POST /api/webhook/settings: scale worker server success")
	}

	// Reauthenticate with Ably
	if req.AblyApiKey != "" {
		util.LOGGER.Info("POST /api/webhook/settings: detect new Ably API key, start reauthentication")

		if err := server.ablyService.Reauthenticate(req.AblyApiKey); err != nil {
			util.LOGGER.Error("POST /api/webhook/settings: failed to reauthenticate Ably service", "error", err)
			errMsgs = append(errMsgs, "failed to reauthenticate Ably service")
		}

		util.LOGGER.Info("POST /api/webhook/settings: reauthenticate Ably success")
	}

	// Reauthenticate email service
	if req.Email != "" || req.AppPassword != "" {
		email, password := server.config.Email, server.config.AppPassword

		if req.Email != "" {
			email = req.Email
		}

		if req.AppPassword != "" {
			password = req.AppPassword
		}

		util.LOGGER.Info("POST /api/webhook/settings: detect changes with email setting, start reauthentication")

		server.mailService.Reauthenticate(email, password)

		util.LOGGER.Info("POST /api/webhook/settings: reauthenticate email service success")
	}

	// Reauthenticate Telegram bot service
	if req.TelegramBotToken != "" {
		util.LOGGER.Info("POST /api/webhook/settings: detect changes with Telegram bot token, start reauthenticate")

		server.bot.Reauthenticate(server.config.DockerTelegramDomain, req.TelegramBotToken)

		util.LOGGER.Info("POST /api/webhook/settings: reauthenticate Telegram bot service success")
	}

	// Reauthenticate Stripe service
	if req.StripeSecretKey != "" {
		util.LOGGER.Info("POST /api/webhook/settings: detect Stripe secret key changes, start reauthentication")

		payment.InitStripe(req.SecretKey)

		util.LOGGER.Info("POST /api/webhook/settings: reauthenticate Stripe service success")
	}

	// Update the config with new system
	server.config.Setting = req
	ctx.JSON(http.StatusOK, SuccessMessage{"Update configuration success"})
}
