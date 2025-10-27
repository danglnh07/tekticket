package worker

import (
	"context"
	"encoding/json"
	"tekticket/db"
	"tekticket/service/notify"
	"tekticket/util"

	"github.com/hibiken/asynq"
)

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
	mailService notify.MailService
}

// Constructor method for Redis task processor
func NewRedisTaskProcessor(
	redisOpts asynq.RedisClientOpt,
	queries *db.Queries,
	mailService notify.MailService,
) TaskProcessor {
	return &RedisTaskProcessor{
		server:      asynq.NewServer(redisOpts, asynq.Config{}),
		queries:     queries,
		mailService: mailService,
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
			util.LOGGER.Error("failed to process task", "task", SendVerifyEmail, "error", err)
			return err
		}

		err := processor.SendVerifyEmail(payload)
		if err != nil {
			util.LOGGER.Error("failed to process task", "task", SendVerifyEmail, "error", err)
			return err
		}

		util.LOGGER.Info("task success", "task", SendVerifyEmail)
		return nil
	})

	return processor.server.Start(mux)
}
