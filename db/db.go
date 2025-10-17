package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
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
	return queries.DB.AutoMigrate(&User{})
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
	return "", fmt.Errorf("cache miss")
}

// ----- User repository methods -----

// CreateUser inserts a new user record
func (queries *Queries) CreateUser(ctx context.Context, user *User) error {
	return queries.DB.WithContext(ctx).Create(user).Error
}

// GetUserByEmail fetches a user by email
func (queries *Queries) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	if err := queries.DB.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByID fetches a user by id
func (queries *Queries) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	var user User
	if err := queries.DB.WithContext(ctx).Where("id = ?", id).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdateUser saves the user entity
func (queries *Queries) UpdateUser(ctx context.Context, user *User) error {
	return queries.DB.WithContext(ctx).Save(user).Error
}

// IncrementTokenVersion bumps token_version for a user
func (queries *Queries) IncrementTokenVersion(ctx context.Context, id uuid.UUID) error {
	return queries.DB.WithContext(ctx).Model(&User{}).Where("id = ?", id).UpdateColumn("token_version", gorm.Expr("token_version + 1")).Error
}
