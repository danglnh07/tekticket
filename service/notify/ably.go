package notify

import (
	"context"

	"github.com/ably/ably-go/ably"
)

// Notification payload
type NotificationPayload struct {
	Channel string `json:"channel"`
	Name    string `json:"name"`  // Event name
	Title   string `json:"title"` // Notification title
	Body    string `json:"body"`  // Notification body (can be HTML, markdown or plain text)
}

// Ably implementation
type AblyService struct {
	client *ably.REST
}

// Ably constructor
func NewAblyService(apiKey string) (*AblyService, error) {
	client, err := ably.NewREST(ably.WithKey(apiKey))
	if err != nil {
		return nil, err
	}

	return &AblyService{client: client}, nil
}

// Publish message to a channel
func (service *AblyService) Publish(ctx context.Context, payload NotificationPayload) error {
	// Get channel
	channel := service.client.Channels.Get(payload.Channel)

	// Build message
	message := &ably.Message{
		Name: payload.Name,
		Data: payload,
	}

	// Publish message
	return channel.Publish(ctx, payload.Name, message)
}
