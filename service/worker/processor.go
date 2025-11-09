package worker

import (
	"context"
	"encoding/json"
	"tekticket/db"
	"tekticket/service/bot"
	"tekticket/service/notify"
	"tekticket/service/uploader"
	"tekticket/util"

	"github.com/hibiken/asynq"
)

var Queues = map[string]int{
	"low":      1,
	"default":  3,
	"critical": 6,
}

const (
	LOW_IMPACT    = "low"
	MEDIUM_IMPACT = "default"
	HIGH_IMPACT   = "critical"
)

func IsQueueLevelExists(queue string) bool {
	_, ok := Queues[queue]
	return ok
}

// Task processor interface
type TaskProcessor interface {
	Start() error
}

// Redis task processor
type RedisTaskProcessor struct {
	// Asynq server
	server *asynq.Server

	// Dependencies
	queries *db.Queries

	// Dependencies
	mailService   notify.MailService
	ablyService   *notify.AblyService
	bot           *bot.Chatbot
	uploadService *uploader.Uploader

	// Config
	config *util.Config
}

// Constructor method for Redis task processor
func NewRedisTaskProcessor(
	redisOpts asynq.RedisClientOpt,
	queries *db.Queries,
	mailService notify.MailService,
	uploadService *uploader.Uploader,
	ablyService *notify.AblyService,
	bot *bot.Chatbot,
	config *util.Config,
) TaskProcessor {
	return &RedisTaskProcessor{
		server:        asynq.NewServer(redisOpts, asynq.Config{Queues: Queues}),
		queries:       queries,
		mailService:   mailService,
		uploadService: uploadService,
		ablyService:   ablyService,
		bot:           bot,
		config:        config,
	}
}

// Method to start the worker server
func (processor *RedisTaskProcessor) Start() error {
	mux := asynq.NewServeMux()

	// Setup handler
	mux.HandleFunc(SendVerifyEmail, func(ctx context.Context, t *asynq.Task) error {
		// Unmarshal payload
		var payload SendVerifyEmailPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			util.LOGGER.Error("failed to unmarshal task's payload", "task", SendVerifyEmail, "error", err)
			return err
		}

		// Process
		if err := processor.SendVerifyEmail(payload); err != nil {
			util.LOGGER.Error("failed to process task", "task", SendVerifyEmail, "error", err)
			return err
		}

		util.LOGGER.Info("task success", "task", SendVerifyEmail)
		return nil
	})

	mux.HandleFunc(SendResetPassword, func(ctx context.Context, t *asynq.Task) error {
		// Unmarshal payload
		var payload SendResetPasswordPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			util.LOGGER.Error("failed to unmarshal task's payload", "task", SendResetPassword, "error", err)
			return err
		}

		// Process
		if err := processor.SendResetPassword(payload); err != nil {
			util.LOGGER.Error("failed to process task", "task", SendResetPassword, "error", err)
			return err
		}

		util.LOGGER.Info("task success", "task", SendResetPassword)
		return nil

	})

	mux.HandleFunc(SendEmailNotification, func(ctx context.Context, t *asynq.Task) error {
		// Unmarshal payload
		var payload SendNotificationPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			util.LOGGER.Error("failed to unmarshal task's payload", "task", SendEmailNotification, "error", err)
			return err
		}

		// Process
		if err := processor.SendEmailNotification(payload.Dest.Email, payload.Title, payload.Body); err != nil {
			util.LOGGER.Error("failed to process task", "task", SendEmailNotification, "error", err)
			return err
		}

		util.LOGGER.Info("task success", "task", SendEmailNotification)
		return nil
	})

	mux.HandleFunc(SendInAppNotification, func(ctx context.Context, t *asynq.Task) error {
		// Unmarshal payload
		var payload SendNotificationPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			util.LOGGER.Error("failed to unmarshal task's payload", "task", SendInAppNotification, "error", err)
			return err
		}

		// Process
		if err := processor.SendInAppNotification(ctx, payload.Dest.Channel, payload.Name, payload.Title, payload.Body); err != nil {
			util.LOGGER.Error("failed to process task", "task", SendInAppNotification, "error", err)
			return err
		}

		util.LOGGER.Info("task success", "task", SendInAppNotification)
		return nil
	})

	mux.HandleFunc(SendTelegramNotification, func(ctx context.Context, t *asynq.Task) error {
		// Unmarshal payload
		var payload SendNotificationPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			util.LOGGER.Error("failed to unmarshal task's payload", "task", SendTelegramNotification, "error", err)
			return err
		}

		// Process
		if err := processor.SendTelegramNotification(payload.Dest.ChatID, payload.Title, payload.Body); err != nil {
			util.LOGGER.Error("failed to process task", "task", SendTelegramNotification, "error", err)
			return err
		}

		util.LOGGER.Info("task success", "task", SendTelegramNotification)
		return nil
	})

	mux.HandleFunc(PublishQRTicket, func(ctx context.Context, t *asynq.Task) error {
		// Unmarshal payload
		var payload PublishQRTicketPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			util.LOGGER.Error("failed to process task", "task", PublishQRTicket, "error", err)
			return err
		}

		err := processor.PublishQRTicket(payload)
		if err != nil {
			util.LOGGER.Error("failed to process task", "task", PublishQRTicket, "error", err)
			return err
		}

		util.LOGGER.Info("task success", "task", PublishQRTicket)
		return nil

	})

	return processor.server.Start(mux)
}
