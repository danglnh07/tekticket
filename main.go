package main

import (
	"context"
	"os"
	"tekticket/api"
	"tekticket/db"
	"tekticket/service/cloudinary"
	"tekticket/service/security"
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
	if err := queries.ConnectDB(config.DbConn); err != nil {
		util.LOGGER.Error("Error connecting to database", "error", err)
		os.Exit(1)
	}

	// Run database migration
	if err := queries.AutoMigration(); err != nil {
		util.LOGGER.Error("Error running auto migration", "error", err, "redis", config.RedisAddr)
		util.LOGGER.Error("RedisAddr", "val", config.RedisAddr)
		os.Exit(1)
	}

	// Connect Redis
	ctx := context.Background()
	if err := queries.ConnectRedis(ctx, &redis.Options{Addr: config.RedisAddr}); err != nil {
		util.LOGGER.Error("Error connecting to Redis", "error", err)
		os.Exit(1)
	}

	// Create dependencies for server
	jwtService := security.NewJWTService(config.SecretKey, config.TokenExpiration, config.RefreshTokenExpiration)
	distributor := worker.NewRedisTaskDistributor(asynq.RedisClientOpt{Addr: config.RedisAddr})
	cld, err := cloudinary.NewCld(config.CloudName, config.CloudKey, config.CloudSecret)
	if err != nil {
		util.LOGGER.Error("failed to initialize Cloudinary service", "error", err)
		os.Exit(1)
	}

	// Start the background server in separate goroutine (since it's will block the main thread)
	go StartBackgroundProcessor(asynq.RedisClientOpt{Addr: config.RedisAddr}, queries)

	// Start server
	server := api.NewServer(queries, jwtService, distributor, cld)
	if err := server.Start(); err != nil {
		util.LOGGER.Error("Failed to start server", "error", err)
		os.Exit(1)
	}

}

func StartBackgroundProcessor(
	redisOpts asynq.RedisClientOpt,
	queries *db.Queries,
) error {
	// Create the processor
	processor := worker.NewRedisTaskProcessor(redisOpts, queries)

	// Start process tasks
	return processor.Start()
}
