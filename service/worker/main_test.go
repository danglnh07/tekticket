package worker

import (
	"context"
	"os"
	"strings"
	"tekticket/db"
	"tekticket/service/notify"
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
	// This integration test shouldn't be run in CI to avoid spamming
	if strings.TrimSpace(os.Getenv("CI")) != "" {
		util.LOGGER.Warn("CI environment, skip integration test")
		return
	}

	queries := db.NewQueries()
	err := queries.ConnectRedis(ctx, &redis.Options{
		Addr: os.Getenv("REDIS_ADDR"),
	})

	if err != nil {
		util.LOGGER.Error("failed to connect to Redis for testing", "error", err)
		os.Exit(1)
	}

	mailService := notify.NewEmailService(os.Getenv("EMAIL"), os.Getenv("APP_PASSWORD"))

	config := &util.Config{
		ResetPasswordURL: "http://localhost:3000", // Just some dump value, we only care about the token in this test
	}
	processor = NewRedisTaskProcessor(asynq.RedisClientOpt{Addr: os.Getenv("REDIS_ADDR")}, queries, mailService, config)
	os.Exit(m.Run())
}
