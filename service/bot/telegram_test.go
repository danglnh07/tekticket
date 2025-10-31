package bot

import (
	"fmt"
	"os"
	"tekticket/util"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	bot *Chatbot
	err error

	scope = map[string]any{"type": "default"}
	lang  = "en"
)

func TestMain(m *testing.M) {
	// If CI environment, skip
	if os.Getenv("CI") != "" {
		util.LOGGER.Warn("CI environment, skip integration test")
		os.Exit(0)
	}

	// Get bot configuration
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	webhook := fmt.Sprintf("%s/api/bot/webhook", os.Getenv("WEBHOOK_DOMAIN"))
	util.LOGGER.Info("Bot info", "token", token, "webhook", webhook)

	// Initialize chatbot
	bot, err = NewChatbot(token, webhook)
	if err != nil {
		util.LOGGER.Error("Failed to initialize chatbot", "error", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestGetInfo(t *testing.T) {
	info, err := bot.GetInfo()
	require.NoError(t, err)
	require.NotNil(t, info)
	util.LOGGER.Info("Bot info", "id", info.ID, "first name", info.FirstName, "username", info.Username)
}

func TestDeleteCommands(t *testing.T) {
	err := bot.DeleteCommands(scope, lang)
	require.NoError(t, err)
}

func TestSetCommands(t *testing.T) {
	// Create random command
	commands := []Command{
		{
			Command:     "/test1",
			Description: "description-1",
		},
		{
			Command:     "/test2",
			Description: "description-2",
		},
		{
			Command:     "/test3",
			Description: "description-3",
		},
	}

	// Create commands
	err := bot.SetCommands(commands, scope, lang)
	require.NoError(t, err)
}

func TestGetCommands(t *testing.T) {
	commands, err := bot.GetCommands(scope, lang)
	require.NoError(t, err)
	require.NotEmpty(t, commands)

	// Try printing
	for _, cmd := range commands {
		util.LOGGER.Info("Command info", "command", cmd.Command, "description", cmd.Description)
	}
}
