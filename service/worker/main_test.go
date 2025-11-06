package worker

import (
	"context"
	"os"
	"strings"
	"tekticket/db"
	"tekticket/service/notify"
	"tekticket/service/uploader"
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
	cld, err := uploader.NewCld(os.Getenv("CLOUDINARY_NAME"), os.Getenv("CLOUDINARY_APIKEY"), os.Getenv("CLOUDINARY_APISECRET"))
	if err != nil {
		util.LOGGER.Error("failed to create cloudinary service", "error", err)
		os.Exit(1)
	}
	util.LOGGER.Info(
		"env",
		"directus addr", os.Getenv("DIRECTUS_ADDR"),
		"static token", os.Getenv("DIRECTUS_STATIC_TOKEN"),
		"secret key", os.Getenv("SECRET_KEY"),
	)
	config := &util.Config{
		ResetPasswordURL:    "http://localhost:3000", // Just some dump value, we only care about the token in this test
		DirectusAddr:        os.Getenv("DIRECTUS_ADDR"),
		DirectusStaticToken: os.Getenv("DIRECTUS_STATIC_TOKEN"),
		SecretKey:           os.Getenv("SECRET_KEY"),
	}

	uploadService := uploader.NewUploader(cld, config)

	processor = NewRedisTaskProcessor(asynq.RedisClientOpt{Addr: os.Getenv("REDIS_ADDR")}, queries, mailService, uploadService, config)
	os.Exit(m.Run())
}
