package worker

type PublishQRTicketPayload struct {
	BookingItemID string `json:"booking_item_id"`
	CheckInURL    string `json:"checkin_url"`
}

const PublishQRTicket = "publish-qr-ticket"

func (processor *RedisTaskProcessor) PublishQRTicket(payload PublishQRTicketPayload) error {
	return nil
}
