// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
package main

import (
	"context"
	"os"
	"tekticket/api"
	"tekticket/db"
	"tekticket/service/notify"
	"tekticket/service/uploader"
	"tekticket/service/worker"
	"tekticket/util"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

func main() {
	// Load config
	config := util.LoadConfig(".env")

	util.LOGGER.Error("RedisAddr", "val", config.RedisAddr)

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
	cld, err := uploader.NewCld(config.CloudName, config.CloudKey, config.CloudSecret)
	if err != nil {
		util.LOGGER.Error("failed to initialize uploader service", "error", err)
		os.Exit(1)
	}
	mailService := notify.NewEmailService(config.Email, config.AppPassword)

	// Start the background server in separate goroutine (since it's will block the main thread)
	go StartBackgroundProcessor(asynq.RedisClientOpt{Addr: config.RedisAddr}, queries, mailService)
	// Start server
	server := api.NewServer(queries, distributor, mailService, cld, config)
	if err := server.Start(); err != nil {
		util.LOGGER.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}

func StartBackgroundProcessor(
	redisOpts asynq.RedisClientOpt,
	queries *db.Queries,
	mailService notify.MailService,
) error {
	// Create the processor
	processor := worker.NewRedisTaskProcessor(redisOpts, queries, mailService)

	// Start process tasks
	return processor.Start()
}
