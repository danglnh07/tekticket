package worker

import (
	"tekticket/db"
	"tekticket/util"
)

type UpdatePaymentRecordPayload struct {
	URL     string         `json:"url"`
	Body    map[string]any `json:"body"`
	Token   string         `json:"token"`
	Caller  string         `json:"caller"`  // The API endpoint that issue this task, used simply for logging
	Context string         `json:"context"` // The context of why/when this task is issued, used simply for logging
}

const UpdatePaymentRecord = "update-payment-record"

// This method is for database rollback when payment (create, confirm, refund) failed
func (processor *RedisTaskProcessor) RetryUpdatePaymentRecord(payload UpdatePaymentRecordPayload) error {
	// Log start
	util.LOGGER.Info("background log", "task", UpdatePaymentRecord, "caller", payload.Caller, "context", payload.Context)

	// Make request
	status, err := db.MakeRequest("PATCH", payload.URL, payload.Body, payload.Token, nil)
	if err != nil {
		util.LOGGER.Error(
			"background log info",
			"task", UpdatePaymentRecord,
			"caller", payload.Caller,
			"context", payload.Context,
			"status", status,
			"error", err,
		)
		return err
	}

	util.LOGGER.Info(
		"background log",
		"task", UpdatePaymentRecord,
		"caller", payload.Caller,
		"context", payload.Context,
		"status", "success",
	)

	return nil
}
