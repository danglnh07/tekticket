package api

import (
	"net/http"
	"tekticket/service/bot"
	"tekticket/util"

	"github.com/gin-gonic/gin"
)

func (server *Server) TelegramWebhook(ctx *gin.Context) {
	// Get the update request
	var req bot.TelegramUpdate
	if err := ctx.ShouldBindJSON(&req); err != nil {
		util.LOGGER.Error("POST /api/webhook/telegram: failed to parse incoming request", "error", err)
		// Telegram may not care what you return, but just add this for testing
		ctx.JSON(http.StatusBadRequest, ErrorResponse{"Invalid request body"})
		return
	}

	// Try printing the chat ID and message content
	util.LOGGER.Info("Telegram webhook update", "chat ID", req.Message.Chat.ID)
	util.LOGGER.Info("Telegram webhook update", "message", req.Message.Text)
}
