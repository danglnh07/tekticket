package util

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config struct
type Config struct {
	DbConn string
	// Neon PG* variables (optional if DbConn provided)
	PGHost                 string
	PGDatabase             string
	PGUser                 string
	PGPassword             string
	PGSSLMode              string
	PGChannelBinding       string
	RedisAddr              string
	SecretKey              []byte
	TokenExpiration        time.Duration
	RefreshTokenExpiration time.Duration
}

func LoadConfig(path string) *Config {
	err := godotenv.Load(path)
	_ = err // ignore error if .env missing; we'll read from env directly

	// Try get and parse data
	tokenExpiration, err := strconv.Atoi(os.Getenv("TOKEN_EXPIRATION"))
	if err != nil {
		// Fallback to default value (60 minutes)
		tokenExpiration = 60
	}

	refreshTokenExpiration, err := strconv.Atoi(os.Getenv("REFRESH_TOKEN_EXPIRATION"))
	if err != nil {
		// Fallback to default value (1440 minutes = 24 hours)
		refreshTokenExpiration = 1440
	}

	cfg := &Config{
		DbConn:                 os.Getenv("DB_CONN"),
		PGHost:                 os.Getenv("PGHOST"),
		PGDatabase:             os.Getenv("PGDATABASE"),
		PGUser:                 os.Getenv("PGUSER"),
		PGPassword:             os.Getenv("PGPASSWORD"),
		PGSSLMode:              os.Getenv("PGSSLMODE"),
		PGChannelBinding:       os.Getenv("PGCHANNELBINDING"),
		RedisAddr:              os.Getenv("REDIS_ADDR"),
		SecretKey:              []byte(os.Getenv("SECRET_KEY")),
		TokenExpiration:        time.Minute * time.Duration(tokenExpiration),
		RefreshTokenExpiration: time.Minute * time.Duration(refreshTokenExpiration),
	}

	// Build DB_CONN from Neon PG* vars if not provided
	if cfg.DbConn == "" && cfg.PGHost != "" && cfg.PGDatabase != "" && cfg.PGUser != "" {
		// Example DSN: host=... user=... password=... dbname=... sslmode=require options=channel_binding=require
		dsn := "host=" + cfg.PGHost +
			" user=" + cfg.PGUser +
			" password=" + cfg.PGPassword +
			" dbname=" + cfg.PGDatabase
		if cfg.PGSSLMode != "" {
			dsn += " sslmode=" + cfg.PGSSLMode
		}
		if cfg.PGChannelBinding != "" {
			dsn += " options=channel_binding=" + cfg.PGChannelBinding
		}
		cfg.DbConn = dsn
	}

	// Default Redis addr if missing
	if cfg.RedisAddr == "" {
		cfg.RedisAddr = "localhost:6379"
	}

	return cfg
}
