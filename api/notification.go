package api

import (
	"fmt"
	"strings"
	"tekticket/db"
	"tekticket/service/bot"
	"tekticket/service/payment"
	"tekticket/util"

	"github.com/gin-gonic/gin"
)

type UserTelegram struct {
	ID             string `json:"id"`
	TelegramChatID string `json:"telegram_chat_id"`
	UserID         string `json:"user_id"`
}

func (server *Server) TelegramWebhook(ctx *gin.Context) {
	// Instead of return a JSON using ctx.JSON, we'll send the message back to client

	// Get the update request
	var req bot.TelegramUpdate
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Error("POST /api/webhook/telegram: failed to parse incoming request", "error", err)
		// If we failed to even get the request message, then we cannot get the chatID -> cannot send message back.
		// So in this step, we should return here
		return
	}

	// Send chat action indicate we are processing
	if err := server.bot.SendChatAction(req.Message.Chat.ID, bot.CHAT_ACTION); err != nil {
		util.LOGGER.Error("POST /api/webhook/telegram: failed to send chat action", "error", err)
	}

	// Try printing the chat ID and message content
	util.LOGGER.Info("Telegram webhook update", "chat ID", req.Message.Chat.ID)
	util.LOGGER.Info("Telegram webhook update", "message", req.Message.Text)

	// Sanitizing text message
	req.Message.Text = strings.TrimSpace(req.Message.Text)

	// Check if this is a Telegram chatbot or just a simple message
	segments := strings.Split(req.Message.Text, " ")
	if len(segments) == 0 {
		util.LOGGER.Warn("POST /api/webhook/telegram: user sent an empty message, ignore this message")
		// Sending a long empty string is technically correct (client can send whatever they want), so we also don't send anything here
		return
	}

	// Act based on the command
	switch {
	case strings.HasPrefix(segments[0], "/start"):
		/*
		 * Command: /start <YOUR_EMAIL> <YOUR_ROLE>
		 */

		// Check if both email and role exists in the command
		if len(segments) != 3 {
			err := server.bot.SendMessage(req.Message.Chat.ID, "<b>LACK ARGUMENTS, MUST PROVIDE BOTH EMAIL AND ROLE</b>")
			if err != nil {
				util.LOGGER.Error("POST /api/webhook/telegram: failed to send message", "error", err)
			}
			return
		}

		// Check if current chat has registered for Telegram service
		var userTelegram []UserTelegram
		url := fmt.Sprintf(
			"%s/items/user_telegrams?fields=id,user_id,telegram_chat_id&filter[telegram_chat_id][_eq]=%d",
			server.config.DirectusAddr,
			req.Message.Chat.ID,
		)
		_, err := db.MakeRequest("GET", url, nil, server.config.DirectusStaticToken, &userTelegram)
		if err != nil {
			util.LOGGER.Error("POST /api/webhook/telegram: failed to check if this chat ID has been registered", "error", err)
			err = server.bot.SendMessage(req.Message.Chat.ID, "<b>INTERNAL SERVER ERROR, PLEASE TRY AGAIN :(</b>")
			if err != nil {
				util.LOGGER.Error("POST /api/webhook/telegram: failed to send message", "error", err)
			}
			return
		}

		if len(userTelegram) != 0 {
			err = server.bot.SendMessage(req.Message.Chat.ID, "You're already registered, did you forget?")
			if err != nil {
				util.LOGGER.Error("POST /api/webhook/telegram: failed to send message", "error", err)
			}
			return
		}

		// Get the list of all users with the provided email
		var users []ProfileResponse // We only need the ID, so any struct that has ID is fined
		url = fmt.Sprintf(
			"%s/users?fields=id&filter[email][_icontains]=%s&filter[role][name][_icontains]=%s",
			server.config.DirectusAddr,
			segments[1],
			segments[2],
		)

		_, err = db.MakeRequest("GET", url, nil, server.config.DirectusStaticToken, &users)
		if err != nil {
			util.LOGGER.Error("POST /api/webhook/telegram: failed to get the list of all users with provided email", "error", err)
			err = server.bot.SendMessage(req.Message.Chat.ID, "<b>INTERNAL SERVER ERROR, PLEASE TRY AGAIN :(</b>")
			if err != nil {
				util.LOGGER.Error("POST /api/webhook/telegram: failed to send message", "error", err)
			}
			return
		}

		// Check if this user exists
		if len(users) == 0 {
			util.LOGGER.Warn("POST /api/webhook/telegram: email not registered", "error", err)
			// Send message back to the client stating that this email is not exists
			err = server.bot.SendMessage(
				req.Message.Chat.ID,
				"<b>THIS EMAIL WITH THIS ROLE IS NOT REGISTERED IN THE SYSTEM! PLEASE TRY ANOTHER</b>",
			)
			if err != nil {
				util.LOGGER.Error("POST /api/webhook/telegram: failed to send message", "error", err)
			}
			return
		}

		// If exists, we add new entry to the user_telegram collections
		url = fmt.Sprintf("%s/items/user_telegrams", server.config.DirectusAddr)
		_, err = db.MakeRequest("POST", url, map[string]any{
			"telegram_chat_id": fmt.Sprintf("%d", req.Message.Chat.ID),
			"user_id":          users[0].ID,
		}, server.config.DirectusStaticToken, nil)

		if err != nil {
			util.LOGGER.Error("POST /api/webhook/telegram: failed to create instance in user_telegram collection", "error", err)
			err = server.bot.SendMessage(req.Message.Chat.ID, "<b>INTERNAL SERVER ERROR! WE ARE SORRY, PLEASE TRY AGAIN</b>")
			if err != nil {
				util.LOGGER.Error("POST /api/webhook/telegram: failed to send message", "error", err)
			}
			return
		}

		err = server.bot.SendMessage(req.Message.Chat.ID, "Success, now you can start receiving my notification :)")
		if err != nil {
			util.LOGGER.Error("POST /api/webhook/telegram: failed to send message", "error", err)
		}
	default:
		// Create a payment method
		pm, err := payment.CreatePaymentMethodFromToken("tok_visa")
		if err != nil {
			util.LOGGER.Error("POST /api/webhook/telegram: failed to create payment method", "error", err)
			server.bot.SendMessage(req.Message.Chat.ID, "Internal server error: "+err.Error())
		} else {
			server.bot.SendMessage(req.Message.Chat.ID, "Payment method ID: "+pm.ID)
		}

		// err = server.bot.SendMessage(req.Message.Chat.ID, "This is an echo message hehe: "+req.Message.Text)
		// if err != nil {
		// 	util.LOGGER.Error("POST /api/webhook/telegram: failed to send message", "error", err)
		// }
	}
}
