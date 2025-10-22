package util

import (
	"os"

	"github.com/joho/godotenv"
)

// Config struct
type Config struct {
	RedisAddr           string
	DirectusAddr        string
	DirectusStaticToken string
	Email               string
	AppPassword         string

	// cloudinary
	CloudName   string
	CloudKey    string
	CloudSecret string
}

func LoadConfig(path string) *Config {
	err := godotenv.Load(path)
	if err != nil {
		return &Config{
			RedisAddr:           os.Getenv("REDIS_ADDR"),
			DirectusAddr:        os.Getenv("DIRECTUS_ADDR"),
			DirectusStaticToken: os.Getenv("DIRECTUS_STATIC_TOKEN"),
			Email:               os.Getenv("EMAIL"),
			AppPassword:         os.Getenv("APP_PASSWORD"),

			// cloudinary
			CloudName:   os.Getenv("CLOUDINARY_NAME"),
			CloudKey:    os.Getenv("CLOUDINARY_APIKEY"),
			CloudSecret: os.Getenv("CLOUDINARY_APISECRET"),
		}
	}

	return &Config{
		RedisAddr:           os.Getenv("REDIS_ADDR"),
		DirectusAddr:        os.Getenv("DIRECTUS_ADDR"),
		DirectusStaticToken: os.Getenv("DIRECTUS_STATIC_TOKEN"),

		Email:       os.Getenv("EMAIL"),
		AppPassword: os.Getenv("APP_PASSWORD"),
		// cloudinary
		CloudName:   os.Getenv("CLOUDINARY_NAME"),
		CloudKey:    os.Getenv("CLOUDINARY_APIKEY"),
		CloudSecret: os.Getenv("CLOUDINARY_APISECRET"),
	}
}
