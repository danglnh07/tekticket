package util

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config struct
type Config struct {
	DbConn                 string
	RedisAddr              string
	SecretKey              []byte
	Email                  string
	AppPassword            string
	TokenExpiration        time.Duration
	RefreshTokenExpiration time.Duration
}

func LoadConfig(path string) *Config {
	err := godotenv.Load(path)
	if err != nil {
		return &Config{
			DbConn:                 os.Getenv("DB_CONN"),
			RedisAddr:              os.Getenv("REDIS_ADDR"),
			SecretKey:              []byte(os.Getenv("SECRET_KEY")),
			Email:                  os.Getenv("EMAIL"),
			AppPassword:            os.Getenv("APP_PASSWORD"),
			TokenExpiration:        time.Minute * 60,
			RefreshTokenExpiration: time.Minute * 1440,
		}
	}

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

	return &Config{
		DbConn:                 os.Getenv("DB_CONN"),
		RedisAddr:              os.Getenv("REDIS_ADDR"),
		SecretKey:              []byte(os.Getenv("SECRET_KEY")),
		Email:                  os.Getenv("EMAIL"),
		AppPassword:            os.Getenv("APP_PASSWORD"),
		TokenExpiration:        time.Minute * time.Duration(tokenExpiration),
		RefreshTokenExpiration: time.Minute * time.Duration(refreshTokenExpiration),
	}
}
