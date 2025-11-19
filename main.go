// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
package main

import (
	"context"
	"fmt"
	"os"
	"tekticket/api"
	"tekticket/db"
	"tekticket/service/bot"
	"tekticket/service/notify"
	"tekticket/service/payment"
	"tekticket/service/uploader"
	"tekticket/service/worker"
	"tekticket/util"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()

	// Load config
	config := util.NewConfig()
	if err := config.LoadStaticConfig(".env"); err != nil {
		util.LOGGER.Warn("Failed to load static config from .env", "error", err)
		util.LOGGER.Warn("Start using environment variables instead")
	}
	if err := config.LoadDynamicConfig(); err != nil {
		util.LOGGER.Error("Failed to load dynamic config from Directus collection", "error", err)
		os.Exit(1)
	}

	// Connect Redis
	queries := db.NewQueries()
	if err := queries.ConnectRedis(ctx, &redis.Options{Addr: config.RedisAddr}); err != nil {
		util.LOGGER.Error("Error connecting to Redis", "error", err)
		os.Exit(1)
	}

	// Background task distributor
	redisClientOpts := asynq.RedisClientOpt{
		Addr: config.RedisAddr,
	}
	distributor := worker.NewRedisTaskDistributor(redisClientOpts)

	// Upload service
	uploadService := uploader.NewUploader(config.DirectusAddr, config.DirectusStaticToken)

	// Mail service
	mailService := notify.NewEmailService(config.Email, config.AppPassword)

	// Telegram bot service
	telegramServer := fmt.Sprintf("%s/bot%s", config.DockerTelegramDomain, config.TelegramBotToken)
	telegramWebhook := fmt.Sprintf("%s/api/webhook/telegram", config.DockerServerDomain)

	bot, err := bot.NewChatbot(telegramServer, telegramWebhook)
	if err != nil {
		util.LOGGER.Error("Failed to initialize Telegram chat bot", "error", err)
		os.Exit(1)
	}

	if err := bot.Setup(); err != nil {
		util.LOGGER.Error("Failed to setup chatbot", "error", err)
		os.Exit(1)
	}

	// Ably service
	ablyService, err := notify.NewAblyService(config.AblyApiKey)
	if err != nil {
		util.LOGGER.Error("Failed to initialize Ably service", "error", err)
		os.Exit(1)
	}

	// Init Stripe service
	payment.InitStripe(config.StripeSecretKey)

	// Background task processor
	processor := worker.NewRedisTaskProcessor(
		asynq.RedisClientOpt{Addr: config.RedisAddr},
		queries,
		mailService,
		uploadService,
		ablyService,
		bot,
		config,
	)

	mux := processor.PrepareHandler()
	if err := processor.Start(mux); err != nil {
		util.LOGGER.Error("failed to start asynq server", "error", err)
		os.Exit(1)
	}

	// Start API server
	server := api.NewServer(queries, distributor, processor, mailService, uploadService, bot, config)
	if err := server.Start(); err != nil {
		util.LOGGER.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}
