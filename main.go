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

	// Connect to database and Redis
	queries := db.NewQueries()
	queries.ConnectDB(config.DirectusAddr, config.DirectusStaticToken)

	// Connect Redis
	ctx := context.Background()
	if err := queries.ConnectRedis(ctx, &redis.Options{Addr: config.RedisAddr}); err != nil {
		util.LOGGER.Error("Error connecting to Redis", "error", err)
		os.Exit(1)
	}

	// Create dependencies for server
	distributor := worker.NewRedisTaskDistributor(asynq.RedisClientOpt{Addr: config.RedisAddr})
	cld, err := uploader.NewCld(config.CloudStorageName, config.CloudStorageKey, config.CloudStorageSecret)
	if err != nil {
		util.LOGGER.Error("failed to initialize uploader service", "error", err)
		os.Exit(1)
	}
	mailService := notify.NewEmailService(config.Email, config.AppPassword)
	bot, err := bot.NewChatbot(config.TelegramBotToken, fmt.Sprintf("%s/api/webhook/telegram", config.ServerDomain))
	if err != nil {
		util.LOGGER.Error("Failed to initialize Telegram chat bot", "error", err)
		os.Exit(1)
	}
	if err := bot.Setup(); err != nil {
		util.LOGGER.Error("Failed to setup chatbot", "error", err)
		os.Exit(1)
	}
	payment.InitStripe(config.StripeSecretKey)

	// Start the background server in separate goroutine (since it's will block the main thread)

	go StartBackgroundProcessor(asynq.RedisClientOpt{Addr: config.RedisAddr}, queries, mailService, config)

	// Start server
	server := api.NewServer(queries, distributor, mailService, cld, bot, config)
	if err := server.Start(); err != nil {
		util.LOGGER.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}

func StartBackgroundProcessor(
	redisOpts asynq.RedisClientOpt,
	queries *db.Queries,
	mailService notify.MailService,
	config *util.Config,
) error {
	// Create the processor
	processor := worker.NewRedisTaskProcessor(redisOpts, queries, mailService, config)

	// Start process tasks
	return processor.Start()
}
