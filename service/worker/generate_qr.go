package worker

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"tekticket/db"
	"tekticket/util"

	"github.com/google/uuid"
)

type PublishQRTicketPayload struct {
	BookingItemIDs []string `json:"booking_item_ids"`
	CheckInURL     string   `json:"checkin_url"`
}

const PublishQRTicket = "publish-qr-ticket"

func (processor *RedisTaskProcessor) generateQRToken(bookingItemID string) (string, error) {
	// Generate token: encrypt AES booking_item_id
	encryption, err := util.Encrypt([]byte(processor.config.SecretKey), []byte(bookingItemID))
	if err != nil {
		return "", err
	}

	// Encode token into base64 URL-safe
	return util.Encode(string(encryption)), nil
}

func VerifyQRToken(token, secretKey string) (string, error) {
	// Decode base64 token
	decode, err := util.Decode(token)
	if err != nil {
		return "", err
	}

	// Decrypt token
	decrypt, err := util.Decrypt([]byte(secretKey), []byte(decode))
	if err != nil {
		return "", err
	}

	return string(decrypt), nil
}

func (processor *RedisTaskProcessor) PublishQRTickets(payload PublishQRTicketPayload) error {
	// Since cloudinary and directus doesn't support batch images upload, we're gonna use goroutine here.
	// While update record is allow for batch update, so we'll only update them at one

	var (
		wg        = sync.WaitGroup{}
		mutex     = sync.Mutex{}
		qrMapping = map[string]string{}
		errs      = make(chan error, len(payload.BookingItemIDs))
	)

	for _, bookingItem := range payload.BookingItemIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()

			// Generate token
			token, err := processor.generateQRToken(id)
			if err != nil {
				// Pour the error into errs channel
				util.LOGGER.Error("failed to generate QR token", "task", PublishQRTicket, "booking_item_id", bookingItem, "error", err)
				errs <- err
				return
			}

			// Create checkin URL
			checkinURL := fmt.Sprintf("%s?token=%s", payload.CheckInURL, token)

			// Generate QR
			qr, err := util.GenerateQR(checkinURL)
			if err != nil {
				util.LOGGER.Error("failed to generate QR", "task", PublishQRTicket, "booking_item_id", bookingItem, "error", err)
				errs <- err
				return
			}

			// Upload image
			respID, status, err := processor.uploadService.Upload(uuid.New().String(), qr)
			if err != nil {
				util.LOGGER.Error(
					"failed to upload QR into cloudinary",
					"task", PublishQRTicket,
					"booking_item_id", bookingItem,
					"status", status,
					"error", err,
				)
				errs <- err
				return
			}

			// Record the mapping payload into the map
			mutex.Lock()
			qrMapping[bookingItem] = respID
			mutex.Unlock()
		}(bookingItem)
	}

	wg.Wait()

	// Check for any error
	close(errs)
	var errorList []error
	for err := range errs {
		errorList = append(errorList, err)
	}

	if len(errorList) > 0 {
		// Build the error message
		errMsg := strings.Builder{}
		for _, err := range errorList {
			errMsg.WriteString(err.Error() + "\n")
		}
		return errors.New(errMsg.String())
	}

	// Update booking_item with new QRs and status
	url := fmt.Sprintf("%s/items/booking_items", processor.config.DirectusAddr)
	body := []map[string]any{}
	for bookingItemID, mappingData := range qrMapping {
		body = append(body, map[string]any{
			"id":     bookingItemID,
			"qr":     mappingData,
			"status": "valid",
		})
	}
	status, err := db.MakeRequest("PATCH", url, body, processor.config.DirectusStaticToken, nil)
	if err != nil {
		util.LOGGER.Error("failed to update booking_item with QR and status", "task", PublishQRTicket, "status", status, "error", err)
		return err
	}

	return nil
}
