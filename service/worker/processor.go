package worker

import (
	"tekticket/db"

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
}

// Constructor method for Redis task processor
func NewRedisTaskProcessor(
	redisOpts asynq.RedisClientOpt,
	queries *db.Queries,
) TaskProcessor {
	return &RedisTaskProcessor{
		server:  asynq.NewServer(redisOpts, asynq.Config{}),
		queries: queries,
	}
}

// Method to start the worker server
func (processor *RedisTaskProcessor) Start() error {
	mux := asynq.NewServeMux()

	// <ADD YOUR HANDLER HERE>

	return processor.server.Start(mux)
}
