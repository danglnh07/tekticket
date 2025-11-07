package util

import (
	"errors"
	"fmt"
	"os"
	"tekticket/db"

	"github.com/joho/godotenv"
)

// Config struct.
// This would include static config that should not changed (or the server must shutdown to update these values)
// and dynamic config (the server can immediatly running the new config)
// Static config is stored in .env, while dynamic config can be accessed via Directus collection: settings
type Config struct {
	// Redis address for background workers
	RedisAddr string
	// Directus URL for making API request to Directus
	DirectusAddr string
	// Used to make request to Directus API that required admin access.
	DirectusStaticToken string
	// Since Directus also depend on Cloudinary for its cloud storage, we can't dynamically configure it
	CloudStorageName     string // Cloudinary cloud name
	CloudStorageKey      string // Cloudinary API key
	CloudStorageSecret   string // Cloudinary secret key
	DockerServerDomain   string // Use for internal service communication
	DockerTelegramDomain string // Use for internal service communication

	// Dynamic config
	Email                string `json:"email"`                  // Platform email
	AppPassword          string `json:"app_password"`           // Platform email's app password
	SecretKey            string `json:"secret_key"`             // Platfrom secret key
	ResetPasswordURL     string `json:"reset_password_url"`     // The frontend URL of the reset password page
	CheckinURL           string `json:"checkin_url"`            // The frontend URL of the checkin page
	StripePublishableKey string `json:"stripe_publishable_key"` // Stripe publishable key
	StripeSecretKey      string `json:"stripe_secret_key"`      // Stripe secret key
	AblyApiKey           string `json:"ably_api_key"`           // Ably API key
	TelegramBotToken     string `json:"telegram_bot_token"`     // Telegram bot token
	ServerDomain         string `json:"server_domain"`          // Server domain, used for external API calling
	MaxWorkers           int    `json:"max_workers"`            // The total of background workers running in the background
	PaymentFeePercent    string `json:"payment_fee_percent"`    // Payment fee percent. Directus will return a string if it a decimal
}

// Constructor method for Config struct
func NewConfig() *Config {
	return &Config{}
}

// Load config from .env
func (config *Config) LoadStaticConfig(path string) error {
	err := godotenv.Load(path)
	if err != nil {
		config.RedisAddr = os.Getenv("REDIS_ADDR")
		config.DirectusAddr = os.Getenv("DIRECTUS_ADDR")
		config.DirectusStaticToken = os.Getenv("DIRECTUS_STATIC_TOKEN")
		config.DockerServerDomain = os.Getenv("DOCKER_SERVER_DOMAIN")
		config.DockerTelegramDomain = os.Getenv("DOCKER_TELEGRAM_DOMAIN")
		return err
	}

	config.RedisAddr = os.Getenv("REDIS_ADDR")
	config.DirectusAddr = os.Getenv("DIRECTUS_ADDR")
	config.DirectusStaticToken = os.Getenv("DIRECTUS_STATIC_TOKEN")
	config.DockerServerDomain = os.Getenv("DOCKER_SERVER_DOMAIN")
	config.DockerTelegramDomain = os.Getenv("DOCKER_TELEGRAM_DOMAIN")

	return nil
}

// Load config from Directus collection. Since this will need both DirectusAddr and DirectusStaticToken,
// make sure to run the config.LoadStaticConfig() first
func (config *Config) LoadDynamicConfig() error {
	// Make request to Directus
	url := fmt.Sprintf("%s/items/settings?filter[in_used][_eq]=true", config.DirectusAddr)
	var configs []Config
	_, err := db.MakeRequest("GET", url, nil, config.DirectusStaticToken, &configs)
	if err != nil {
		return err
	}

	if len(configs) == 0 {
		return errors.New("service has no configurations active, cannot start running")
	}

	// Fill config with values fetched from Directus
	config.Email = configs[0].Email
	config.AppPassword = configs[0].AppPassword
	config.SecretKey = configs[0].SecretKey
	config.ResetPasswordURL = configs[0].ResetPasswordURL
	config.CheckinURL = configs[0].CheckinURL
	config.StripePublishableKey = configs[0].StripePublishableKey
	config.StripeSecretKey = configs[0].StripeSecretKey
	config.AblyApiKey = configs[0].AblyApiKey
	config.TelegramBotToken = configs[0].TelegramBotToken
	config.ServerDomain = configs[0].ServerDomain
	config.MaxWorkers = configs[0].MaxWorkers
	config.PaymentFeePercent = configs[0].PaymentFeePercent

	return nil
}
