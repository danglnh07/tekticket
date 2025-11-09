package worker

import (
	"bytes"
	"context"
	"fmt"
	"tekticket/db"
	"tekticket/util"
)

type PublishQRTicketPayload struct {
	BookingItemID string `json:"booking_item_id"`
	CheckInURL    string `json:"checkin_url"`
}

const PublishQRTicket = "publish-qr-ticket"

func (processor *RedisTaskProcessor) PublishQRTicket(payload PublishQRTicketPayload) error {
	// Generate token: encrypt AES booking_item_id
	encryption, err := util.Encrypt([]byte(processor.config.SecretKey), []byte(payload.BookingItemID))
	if err != nil {
		return err
	}

	// Append the token into checkinURL
	payload.CheckInURL += "?token=" + util.Encode(string(encryption))
	util.LOGGER.Info("Check in URL with token parameter", "task", PublishQRTicket, "checkin_url", payload.CheckInURL)

	// Decode into QR
	qr, err := util.GenerateQR(payload.CheckInURL)
	if err != nil {
		return err
	}

	// Upload image into cloudinary and directus
	status, id, err := processor.uploadService.UploadImage(context.Background(), bytes.NewReader(qr))
	if err != nil {
		util.LOGGER.Error("failed to upload QR", "task", PublishQRTicket, "status", status, "error", err)
		return err
	}

	util.LOGGER.Info("qr ID Directus", "task", PublishQRTicket, "id", id)

	// Update booking_item with new QR
	url := fmt.Sprintf("%s/items/booking_items/%s", processor.config.DirectusAddr, payload.BookingItemID)
	status, err = db.MakeRequest("PATCH", url, map[string]any{"qr": id, "status": "valid"}, processor.config.DirectusStaticToken, nil)
	if err != nil {
		util.LOGGER.Error("failed to update booking_item with QR and status", "task", PublishQRTicket, "status", status, "error", err)
		return err
	}

	return nil
}
