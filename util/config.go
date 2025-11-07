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
	db.Setting
}

// Constructor method for Config struct
func NewConfig() *Config {
	return &Config{}
}

// Load config from .env
func (config *Config) LoadStaticConfig(path string) error {
	err := godotenv.Load(path)
	if err != nil {
		config.RedisAddr = os.Getenv("DOCKER_REDIS_ADDR")
		config.DirectusAddr = os.Getenv("DOCKER_DIRECTUS_DOMAIN")
		config.DirectusStaticToken = os.Getenv("DIRECTUS_STATIC_TOKEN")
		config.DockerServerDomain = os.Getenv("DOCKER_SERVER_DOMAIN")
		config.DockerTelegramDomain = os.Getenv("DOCKER_TELEGRAM_DOMAIN")
		return err
	}

	config.RedisAddr = os.Getenv("DOCKER_REDIS_ADDR")
	config.DirectusAddr = os.Getenv("DOCKER_DIRECTUS_DOMAIN")
	config.DirectusStaticToken = os.Getenv("DIRECTUS_STATIC_TOKEN")
	config.DockerServerDomain = os.Getenv("DOCKER_SERVER_DOMAIN")
	config.DockerTelegramDomain = os.Getenv("DOCKER_TELEGRAM_DOMAIN")

	return nil
}

// Load config from Directus collection. Since this will need both DirectusAddr and DirectusStaticToken,
// make sure to run the config.LoadStaticConfig() first
func (config *Config) LoadDynamicConfig() error {
	// Get the latest setting that is in used
	url := fmt.Sprintf("%s/items/settings?filter[in_used][_eq]=true&sort=-version", config.DirectusAddr)
	var configs []db.Setting
	_, err := db.MakeRequest("GET", url, nil, config.DirectusStaticToken, &configs)
	if err != nil {
		return err
	}

	if len(configs) == 0 {
		return errors.New("service has no configurations active, cannot start running")
	}

	config.Setting = configs[0]
	LOGGER.Info("Dynamic config", "version", config.Version)

	return nil
}
