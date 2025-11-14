package notify

import (
	"context"

	"github.com/ably/ably-go/ably"
)

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

// Publish message to a channel.
// channelName is the name of the channel to send the message to. It must be correct, or else the other side couldn't get it
// eventName is the name of the event that fire this notification.
// data can be anything, but it should be a structured data contains the notification's title and body
func (service *AblyService) Publish(ctx context.Context, channelName, eventName string, data any) error {
	// Get channel
	channel := service.client.Channels.Get(channelName)

	// Build message
	message := &ably.Message{
		Name: eventName,
		Data: data,
	}

	// Publish message
	return channel.Publish(ctx, eventName, message)
}

// This method is purely for test, it should be the client responsible to fetch this
func (service *AblyService) getMessageHistory(ctx context.Context, channelName string) ([]*ably.Message, error) {
	channel := service.client.Channels.Get(channelName)

	pages, err := channel.History().Pages(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch the first page
	if !pages.Next(ctx) {
		// Check for error
		if err := pages.Err(); err != nil {
			return nil, err
		}

		// No messages available
		return nil, nil
	}

	return pages.Items(), nil
}
