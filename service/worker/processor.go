package worker

import (
	"context"
	"tekticket/db"
	"tekticket/service/mail"

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
	mailService mail.MailService
}

// Constructor method for Redis task processor
func NewRedisTaskProcessor(
	redisOpts asynq.RedisClientOpt,
	queries *db.Queries,
	mailService mail.MailService,
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
		return processor.SendVerifyEmail(t.Payload())
	})

	return processor.server.Start(mux)
}
