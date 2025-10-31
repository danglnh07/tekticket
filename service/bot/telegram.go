package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

/*
 * Telegram official bot API docs: https://core.telegram.org/bots/api
 * NOTE:
 * 1. Telegram bot API did NOT follow REST standard: GET request simply a request without any JSON payload, while POST request is
 * request with payload
 * 2. Telegram allow only 1 webhook attach at the time
 * 3. The response returned always return a boolean field 'ok'.
 * 3.1. If request failed, 2 additional fields is return: 'error_code', which is a HTTP status code, and 'description'
 * 3.2. If request success, an additional field will be return: 'result', which can have whatever value
 */

// Telegram chatbot implementation
type Chatbot struct {
	server  string
	webhook string
}

// Constructor of chatbot, which register the Telegram server domain and setting webhook
func NewChatbot(token, webhook string) (*Chatbot, error) {
	bot := &Chatbot{
		server:  "https://api.telegram.org/bot" + token,
		webhook: webhook,
	}

	return bot, nil
}

// Utility method: GET request
func (bot *Chatbot) Get(path string, result any) error {
	// Make request to Telegram API
	resp, err := http.Get(fmt.Sprintf("%s/%s", bot.server, path))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Parse response
	var data TelegramResponse
	if err = json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}

	// Check response status
	if !data.OK {
		return fmt.Errorf("failed to perform Telegram request (%d: %s)", data.ErrorCode, data.Description)
	}

	// If success, try parsing into result pointer
	if result != nil {
		resultBytes, _ := json.Marshal(data.Result)
		if err := json.Unmarshal(resultBytes, result); err != nil {
			return err
		}
	}

	return nil
}

// Utility method: POST request
func (bot *Chatbot) Post(path string, payload map[string]any, result any) error {
	var body *bytes.Buffer = nil

	// If payload is provided, build the request body
	if payload != nil {
		// Unmarshal payload
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewBuffer(data)
	}

	// Make request to Telegram API
	resp, err := http.Post(fmt.Sprintf("%s/%s", bot.server, path), "application/json", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Parse response
	var data TelegramResponse
	if err = json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}

	// Check response status
	if !data.OK {
		return fmt.Errorf("failed to perform Telegram request (%d: %s)", data.ErrorCode, data.Description)
	}

	// If success, try parsing into result pointer
	if result != nil {
		resultBytes, _ := json.Marshal(data.Result)
		if err := json.Unmarshal(resultBytes, result); err != nil {
			return err
		}
	}

	return nil
}

// Get webhook URL.
func (bot *Chatbot) GetWebhook() (Webhook, error) {
	var webhook Webhook
	if err := bot.Get("getWebhookInfo", &webhook); err != nil {
		return webhook, err
	}
	return webhook, nil
}

// Set webhook URL
func (bot *Chatbot) SetWebhook(url string) error {
	return bot.Post("setWebhook", map[string]any{"url": url}, nil)
}

// Delete a webhook
func (bot *Chatbot) DeleteWebhook() error {
	return bot.Post("deleteWebhook", nil, nil)
}

// Get the current bot information
func (bot *Chatbot) GetInfo() (BotInfo, error) {
	var info BotInfo
	if err := bot.Get("getMe", &info); err != nil {
		return info, err
	}
	return info, nil
}

// Get all bot's commands
func (bot *Chatbot) GetCommands(scope map[string]any, lang string) ([]Command, error) {
	var commands []Command
	err := bot.Post(
		"getMyCommands",
		map[string]any{
			"scope":         scope,
			"language_code": lang,
		},
		&commands,
	)
	if err != nil {
		return nil, err
	}

	return commands, nil
}

// Set bot commands
func (bot *Chatbot) SetCommands(commands []Command, scope map[string]any, lang string) error {
	return bot.Post("setMyCommands", map[string]any{
		"commands":      commands,
		"scope":         scope,
		"language_code": lang,
	}, nil)
}

// Delete bot commands. It will delete ALL commands of the given scope and language
func (bot *Chatbot) DeleteCommands(scope map[string]any, lang string) error {
	return bot.Post("deleteMyCommands", map[string]any{
		"scope":         scope,
		"language_code": lang,
	}, nil)
}
