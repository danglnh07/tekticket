package worker

import (
	"context"
	"fmt"
	"os"
	"strings"
	"tekticket/db"
	"tekticket/service/bot"
	"tekticket/service/notify"
	"tekticket/service/uploader"
	"tekticket/util"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
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
	bot, err := bot.NewChatbot(os.Getenv("TELEGRAM_BOT_TOKEN"), fmt.Sprintf("%s/api/webhook/telegram", os.Getenv("SERVER_DOMAIN")))

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
		Setting: db.Setting{
			ResetPasswordURL: "http://localhost:3000",
			SecretKey:        os.Getenv("SECRET_KEY"),
		},
		DirectusAddr:        os.Getenv("DIRECTUS_ADDR"),
		DirectusStaticToken: os.Getenv("DIRECTUS_STATIC_TOKEN"),
	}
	uploadService := uploader.NewUploader(cld, config)

	processor = NewRedisTaskProcessor(
		asynq.RedisClientOpt{Addr: os.Getenv("REDIS_ADDR")},
		queries,
		mailService,
		uploadService,
		nil,
		bot,
		config,
	)

	os.Exit(m.Run())
}

// Test: send verify email with random OTP
func TestSendVerifyEmail(t *testing.T) {
	// Send email
	err := processor.(*RedisTaskProcessor).SendVerifyEmail(SendVerifyEmailPayload{
		ID:       util.RandomString(12),
		Email:    os.Getenv("RECEIVE_EMAIL"),
		Username: util.RandomString(10),
		OTP:      util.GenerateRandomOTP(),
	})
	require.NoError(t, err)
}

// Test: generate token for reset password.
func TestGenerateResetPasswordToken(t *testing.T) {
	// Generate random test data
	id := uuid.New().String()
	email := util.RandomString(12)
	token, err := processor.(*RedisTaskProcessor).generateResetPasswordToken(id, email)
	require.NoError(t, err)
	require.NotEmpty(t, token)
}

// Test: verify reset password token
func TestVerifyResetPasswordToken(t *testing.T) {
	// Generate random test data
	id := uuid.New().String()
	email := util.RandomString(12)

	// Generate token
	token, err := processor.(*RedisTaskProcessor).generateResetPasswordToken(id, email)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	//  Verify token
	payload, err := VerifyResetPasswordToken(token, processor.(*RedisTaskProcessor).config.SecretKey)
	require.NoError(t, err)
	require.Equal(t, id, payload[0])
	require.Equal(t, email, payload[1])
}

// Test: generate QR token for checkin
func TestGenerateQRToken(t *testing.T) {
	// Generate random test data
	bookingItem := uuid.New().String()

	// Generate token
	token, err := processor.(*RedisTaskProcessor).generateQRToken(bookingItem)
	require.NoError(t, err)
	require.NotEmpty(t, token)
}

// Test: verify QR token
func TestVerifyQRToken(t *testing.T) {
	// Generate random test data
	bookingItem := uuid.New().String()

	// Generate token
	token, err := processor.(*RedisTaskProcessor).generateQRToken(bookingItem)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	// Verify token
	result, err := processor.(*RedisTaskProcessor).VerifyQRToken(token)
	require.NoError(t, err)
	require.Equal(t, bookingItem, result)
}
