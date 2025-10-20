package worker

import (
	"context"
	"os"
	"tekticket/db"
	"tekticket/service/mail"
	"tekticket/util"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

var (
	ctx       = context.Background()
	processor TaskProcessor
)

func TestMain(m *testing.M) {
	queries := db.NewQueries()
	err := queries.ConnectRedis(ctx, &redis.Options{
		Addr: os.Getenv("REDIS_ADDR"),
	})

	if err != nil {
		util.LOGGER.Error("failed to connect to Redis for testing", "error", err)
		os.Exit(1)
	}

	mailService := mail.NewEmailService(os.Getenv("EMAIL"), os.Getenv("APP_PASSWORD"))

	processor = NewRedisTaskProcessor(asynq.RedisClientOpt{Addr: os.Getenv("REDIS_ADDR")}, queries, mailService)
	os.Exit(m.Run())
}
