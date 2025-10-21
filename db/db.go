package db

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// The queries object for interacting with database and cache
type Queries struct {
	DB    *gorm.DB
	Cache *redis.Client
}

// Constructor for Queries
func NewQueries() *Queries {
	return &Queries{}
}

// Connect to Postgres
func (queries *Queries) ConnectDB(connStr string) error {
	conn, err := gorm.Open(postgres.Open(connStr))
	if err != nil {
		return err
	}

	queries.DB = conn
	return nil
}

// Run postgres database auto migration
func (queries *Queries) AutoMigration() error {
	return queries.DB.AutoMigrate()
}

// Connect to Redis
func (queries *Queries) ConnectRedis(ctx context.Context, opt *redis.Options) error {
	queries.Cache = redis.NewClient(opt)
	_, err := queries.Cache.Ping(ctx).Result()
	if err != nil {
		return err
	}
	return nil
}

// Set cache value. If expired = 0, it will set the expiration time to 1 hour instead of no expiration
func (queries *Queries) SetCache(ctx context.Context, key string, val string, expired time.Duration) {
	if expired == 0 {
		expired = time.Hour
	}
	queries.Cache.Set(ctx, key, val, expired)
}

type ErrorCacheMiss struct {
	Message string
}

func (e *ErrorCacheMiss) Error() string {
	return "cache miss"
}

// Get cache value
func (queries *Queries) GetCache(ctx context.Context, key string) (string, error) {
	val, err := queries.Cache.Get(ctx, key).Result()

	// If actually found value, return the val
	if err == nil {
		return val, nil
	}

	// If redis error
	if err != redis.Nil {
		return "", err
	}

	// If the value of the key simply don't exists, or expired
	return "", &ErrorCacheMiss{Message: "cache miss"}
}
