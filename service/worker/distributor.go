package worker

import (
	"context"
	"encoding/json"
	"tekticket/util"

	"github.com/hibiken/asynq"
)

// Task distributor interface
type TaskDistributor interface {
	DistributeTask(ctx context.Context, taskName string, payload any, opts ...asynq.Option) error
}

// Redis task distributor
type RedisTaskDistributor struct {
	client *asynq.Client
}

// Constructor method for Redis task distributor
func NewRedisTaskDistributor(redisOpt asynq.RedisClientOpt) TaskDistributor {
	client := asynq.NewClient(redisOpt)
	return &RedisTaskDistributor{
		client: client,
	}
}

// Distribute task.
// `name` should be unique since it's used to identify task
func (distributor *RedisTaskDistributor) DistributeTask(ctx context.Context, name string, payload any, opts ...asynq.Option) error {
	// Marshal payload
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// Create new task
	task := asynq.NewTask(name, data, opts...)

	// Send task to Redis queue
	info, err := distributor.client.EnqueueContext(ctx, task)
	if err != nil {
		return err
	}

	// Log task info
	util.LOGGER.Info("Task info", "task_name", name, "queue", info.Queue, "max_retry", info.MaxRetry)

	return nil
}
