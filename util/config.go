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
	SecretKey           string
	FrontendURL         string

	// cloudinary
	CloudName   string
	CloudKey    string
	CloudSecret string

	// Stripe
	StripePublishableKey string
	StripeSecretKey      string
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
			SecretKey:           os.Getenv("SECRET_KEY"),
			FrontendURL:         os.Getenv("FRONTEND_URL"),
			// cloudinary
			CloudName:   os.Getenv("CLOUDINARY_NAME"),
			CloudKey:    os.Getenv("CLOUDINARY_APIKEY"),
			CloudSecret: os.Getenv("CLOUDINARY_APISECRET"),

			// Stripe
			StripePublishableKey: os.Getenv("STRIPE_PUBLISHABLE_KEY"),
			StripeSecretKey:      os.Getenv("STRIPE_SECRET_KEY"),
		}
	}

	return &Config{
		RedisAddr:           os.Getenv("REDIS_ADDR"),
		DirectusAddr:        os.Getenv("DIRECTUS_ADDR"),
		DirectusStaticToken: os.Getenv("DIRECTUS_STATIC_TOKEN"),

		Email:       os.Getenv("EMAIL"),
		AppPassword: os.Getenv("APP_PASSWORD"),
		SecretKey:   os.Getenv("SECRET_KEY"),
		FrontendURL: os.Getenv("FRONTEND_URL"),

		// cloudinary
		CloudName:   os.Getenv("CLOUDINARY_NAME"),
		CloudKey:    os.Getenv("CLOUDINARY_APIKEY"),
		CloudSecret: os.Getenv("CLOUDINARY_APISECRET"),

		// Stripe
		StripePublishableKey: os.Getenv("STRIPE_PUBLISHABLE_KEY"),
		StripeSecretKey:      os.Getenv("STRIPE_SECRET_KEY"),
	}
}
