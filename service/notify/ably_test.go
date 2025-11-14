package notify

import (
	"os"
	"tekticket/util"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var (
	service *AblyService
	err     error
	channel = "test-channel"
	event   = "integration-test"
)

func TestMain(m *testing.M) {
	// If CI environment, skip
	if os.Getenv("CI") != "" {
		util.LOGGER.Warn("CI environment, skip integration test")
		os.Exit(0)
	}

	// Get Ably API key and initialize AblyService
	apiKey := os.Getenv("ABLY_API_KEY")
	service, err = NewAblyService(apiKey)
	if err != nil {
		util.LOGGER.Error("failed to initialize ably service for notify package testing")
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestPublish(t *testing.T) {
	// Create test data
	payload := map[string]any{
		"message": util.RandomString(6),
	}

	err = service.Publish(t.Context(), channel, event, payload)
	require.NoError(t, err)
}

func TestGetMessageHistory(t *testing.T) {
	// Create test data
	payload := map[string]any{
		"message": "test-get-message-history",
		"key":     util.RandomString(10),
	}

	// Publish message
	err = service.Publish(t.Context(), channel, event, payload)
	require.NoError(t, err)

	time.Sleep(time.Second * 10)

	// Get message history
	messages, err := service.getMessageHistory(t.Context(), channel)
	require.NoError(t, err)
	require.NotEmpty(t, messages)

	for _, msg := range messages {
		util.LOGGER.Info(
			"Ably history message",
			"id", msg.ID,
			"client_id", msg.ClientID,
			"connection_id", msg.ConnectionID,
			"name", msg.Name,
			"payload", msg.Data)
	}
}
