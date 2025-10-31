package util

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config struct.
// This would include static config that should not changed (or the server must shutdown to update these values)
// and dynamic config (the server can immediatly running the new config)
// Static config is stored in .env, while dynamic config can be accessed via Directus collection: settings
type Config struct {
	/* Static config */

	// Redis address for background workers
	RedisAddr string
	// Directus URL for making API request to Directus
	DirectusAddr string
	// Used to make request to Directus API that required admin access.
	DirectusStaticToken string

	// Dynamic config
	Email                string `json:"email"`                  // Platform email
	AppPassword          string `json:"app_password"`           // Platform email's app password
	SecretKey            string `json:"secret_key"`             // Platfrom secret key
	ResetPasswordURL     string `json:"reset_password_url"`     // The frontend URL of the reset password page
	CheckinURL           string `json:"checkin_url"`            // The frontend URL of the checkin page
	CloudStorageName     string `json:"cloud_storage_name"`     // Cloudinary cloud name
	CloudStorageKey      string `json:"cloud_storage_key"`      // Cloudinary API key
	CloudStorageSecret   string `json:"cloud_storage_secret"`   // Cloudinary secret key
	StripePublishableKey string `json:"stripe_publishable_key"` // Stripe publishable key
	StripeSecretKey      string `json:"stripe_secret_key"`      // Stripe secret key
	AblyApiKey           string `json:"ably_api_key"`           // Ably API key
	TelegramBotToken     string `json:"telegram_bot_token"`     // Telegram bot token
	NgrokAuthToken       string `json:"ngrok_auth_token"`       // Used for Ngrok tunnelling, if using Ngrok
	ServerDomain         string `json:"server_domain"`          // Server domain, it can be Ngrok generated, or a custom domain
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
		return err
	}

	config.RedisAddr = os.Getenv("REDIS_ADDR")
	config.DirectusAddr = os.Getenv("DIRECTUS_ADDR")
	config.DirectusStaticToken = os.Getenv("DIRECTUS_STATIC_TOKEN")
	return nil
}

// Load config from Directus collection. Since this will need both DirectusAddr and DirectusStaticToken,
// make sure to run the config.LoadStaticConfig() first
func (config *Config) LoadDynamicConfig() error {
	// Make request to Directus
	url := fmt.Sprintf("%s/items/settings?filter[in_used][_eq]=true", config.DirectusAddr)
	resp, _, err := MakeRequest("GET", url, nil, config.DirectusStaticToken)
	if err != nil {
		return err
	}

	var directusResp struct {
		Data []Config
	}

	if err := json.NewDecoder(resp.Body).Decode(&directusResp); err != nil {
		return err
	}

	// Fill config with values fetched from Directus
	config.Email = directusResp.Data[0].Email
	config.AppPassword = directusResp.Data[0].AppPassword
	config.SecretKey = directusResp.Data[0].SecretKey
	config.ResetPasswordURL = directusResp.Data[0].ResetPasswordURL
	config.CheckinURL = directusResp.Data[0].CheckinURL
	config.CloudStorageName = directusResp.Data[0].CloudStorageName
	config.CloudStorageKey = directusResp.Data[0].CloudStorageKey
	config.CloudStorageSecret = directusResp.Data[0].CloudStorageSecret
	config.StripePublishableKey = directusResp.Data[0].StripePublishableKey
	config.StripeSecretKey = directusResp.Data[0].StripeSecretKey
	config.AblyApiKey = directusResp.Data[0].AblyApiKey
	config.TelegramBotToken = directusResp.Data[0].TelegramBotToken
	config.NgrokAuthToken = directusResp.Data[0].NgrokAuthToken
	config.ServerDomain = directusResp.Data[0].ServerDomain

	return nil
}
