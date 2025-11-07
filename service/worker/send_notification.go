package worker

import (
	"context"
	"tekticket/service/bot"
	"tekticket/util"
)

type NotificationChannel struct {
	Email   string `json:"email"`   // This is for email notification
	Channel string `json:"channel"` // This is for in app notification
	ChatID  int    `json:"chat_id"` // This is for Telegram notification
}

type SendNotificationPayload struct {
	Name  string              `json:"name"`
	Title string              `json:"title"`
	Body  string              `json:"body"`
	Dest  NotificationChannel `json:"dest"`
}

const (
	SendEmailNotification    = "send-email-notification"
	SendInAppNotification    = "send-inapp-notification"
	SendTelegramNotification = "send-telegram-notification"
)

func (processor *RedisTaskProcessor) SendEmailNotification(email, title, body string) error {
	return processor.mailService.SendEmail(email, title, body)
}

func (processor *RedisTaskProcessor) SendInAppNotification(ctx context.Context, channel, name, title, body string) error {
	return processor.ablyService.Publish(ctx, channel, name, map[string]any{
		"title": title,
		"body":  body,
	})
}

func (processor *RedisTaskProcessor) SendTelegramNotification(chatID int, title, body string) error {
	// Send chat action for more interactive, it's not necessary so we don't care if it's return error or not
	if err := processor.bot.SendChatAction(chatID, bot.CHAT_ACTION); err != nil {
		util.LOGGER.Warn("failed to send chat action for telegram notification", "error", err)
	}

	// Send message to telegram
	return processor.bot.SendMessage(chatID, util.FormatNotificationHTML(title, body))
}
