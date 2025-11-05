package api

import (
	"fmt"
	"net/http"
	"strings"
	"tekticket/service/bot"
	"tekticket/service/worker"
	"tekticket/util"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
)

// Telegram webhook that will listen to any message that user send to the bot.
func (server *Server) TelegramWebhook(ctx *gin.Context) {
	// Get the update request
	var req bot.TelegramUpdate
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Error("POST /api/webhook/telegram: failed to parse incoming request", "error", err)
		return
	}

	chatID := req.Message.Chat.ID
	message := strings.TrimSpace(req.Message.Text)
	util.LOGGER.Info("Receive telegram message", "chat_id", chatID, "message", message)

	// Send chat action indicate we are processing
	if err := server.bot.SendChatAction(chatID, bot.CHAT_ACTION); err != nil {
		util.LOGGER.Error("POST /api/webhook/telegram: failed to send chat action", "error", err)
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
	case "/start":
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
			err := server.bot.SendMessage(chatID, util.FormatWarningHTML("You must provide your email for registration!"))
			if err != nil {
				util.LOGGER.Error("POST /api/webhook/telegram: failed to send message", "error", err)
			}
			return
		}

		// Check if current chat has registered for Telegram service
		if _, err := server.queries.GetCache(ctx, fmt.Sprintf("%d", chatID)); err != nil {
			// Whether this is a cache miss or an actual error, we have no way to determine if the chatID already registered
			// so we'll try to make a request to Directus to check
			var userTelegram []map[string]any // We only care if there are any records, so a map is enough
			url := fmt.Sprintf("%s/items/user_telegrams?fields=*&filter[telegram_chat_id][_eq]=%d", server.config.DirectusAddr, chatID)
			_, err := util.MakeRequest("GET", url, nil, server.config.DirectusStaticToken, &userTelegram)
			if err != nil {
				util.LOGGER.Error("POST /api/webhook/telegram: failed to check if this chat ID has been registered", "error", err)
				err = server.bot.SendMessage(chatID, util.FormatWarningHTML("Internal server error! Please try again"))
				if err != nil {
					util.LOGGER.Error("POST /api/webhook/telegram: failed to send message", "error", err)
				}
				return
			}

			if len(userTelegram) != 0 {
				err = server.bot.SendMessage(chatID, "You're already registered, did you forget?")
				if err != nil {
					util.LOGGER.Error("POST /api/webhook/telegram: failed to send message", "error", err)
				}
				return
			}
		} else {
			// If cache hit -> chatID already registered
			err = server.bot.SendMessage(chatID, "You're already registered, did you forget?")
			if err != nil {
				util.LOGGER.Error("POST /api/webhook/telegram: failed to send message", "error", err)
			}
			return
		}

		// Get the list of all users with the provided email
		var users []ProfileResponse // We only need the ID, so any struct that has ID is fined
		url := fmt.Sprintf(
			"%s/users?fields=id&filter[email][_icontains]=%s&filter[role][name][_icontains]=%s",
			server.config.DirectusAddr,
			segments[1],
			segments[2],
		)

		_, err := util.MakeRequest("GET", url, nil, server.config.DirectusStaticToken, &users)
		if err != nil {
			util.LOGGER.Error("POST /api/webhook/telegram: failed to get the list of all users with provided email", "error", err)
			err = server.bot.SendMessage(chatID, util.FormatWarningHTML("Internal server error! Please try again"))
			if err != nil {
				util.LOGGER.Error("POST /api/webhook/telegram: failed to send message", "error", err)
			}
			return
		}

		// Check if this user exists
		if len(users) == 0 {
			util.LOGGER.Warn("POST /api/webhook/telegram: email not registered", "error", err)
			err = server.bot.SendMessage(chatID, "This email not exists in our system, please try another")
			if err != nil {
				util.LOGGER.Error("POST /api/webhook/telegram: failed to send message", "error", err)
			}
			return
		}

		// If exists, we add new entry to the user_telegram collections
		url = fmt.Sprintf("%s/items/user_telegrams", server.config.DirectusAddr)
		_, err = util.MakeRequest("POST", url, map[string]any{
			"telegram_chat_id": fmt.Sprintf("%d", req.Message.Chat.ID),
			"user_id":          users[0].ID,
		}, server.config.DirectusStaticToken, nil)

		if err != nil {
			util.LOGGER.Error("POST /api/webhook/telegram: failed to create instance in user_telegram collection", "error", err)
			err = server.bot.SendMessage(chatID, util.FormatWarningHTML("Internal server error! Please try again"))
			if err != nil {
				util.LOGGER.Error("POST /api/webhook/telegram: failed to send message", "error", err)
			}
			return
		}

		err = server.bot.SendMessage(req.Message.Chat.ID, "Success, now you can start receiving my notification :)")
		if err != nil {
			util.LOGGER.Error("POST /api/webhook/telegram: failed to send message", "error", err)
		}

		// Store the current chatID into cache
		server.queries.SetCache(ctx, fmt.Sprintf("%d", chatID), "", time.Hour) // The value can be whatever, we don't really care
	default:
		err := server.bot.SendMessage(req.Message.Chat.ID, "This is an echo message hehe: "+req.Message.Text)
		if err != nil {
			util.LOGGER.Error("POST /api/webhook/telegram: failed to send message", "error", err)
		}
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
// @Tags         Notifications
// @Accept       json
// @Produce      json
// @Param        request  body  NotificationRequest  true  "Notification webhook payload"
// @Success      200  {object}  SuccessMessage       "Notification dispatched successfully"
// @Failure      400  {object}  ErrorResponse        "Invalid request body or missing required destinations"
// @Failure      500  {object}  ErrorResponse        "Internal server error or failed to distribute background task"
// @Router       /api/notifications/webhook [post]
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
				"POST /api/notifications/webhook: failed to distribute task",
				"task", worker.SendInAppNotification,
				"error", err,
			)
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
			return
		}
	} else {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"In app notification is required for all notifications"})
		return
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
				"POST /api/notifications/webhook: failed to distribute task",
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
				"POST /api/notifications/webhook: failed to distribute task",
				"task", worker.SendEmailNotification,
				"error", err,
			)
			ctx.JSON(http.StatusInternalServerError, ErrorResponse{"Internal server error"})
			return
		}
	}
}
