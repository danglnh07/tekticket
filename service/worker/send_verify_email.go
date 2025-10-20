package worker

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"tekticket/service/security"
	"time"

	"github.com/google/uuid"
)

type SendVerifyEmailPayload struct {
	ID       uuid.UUID
	Email    string `json:"email"`
	Username string `json:"username"`
	OTP      string `json:"otp"`
}

const SendVerifyEmail = "send-verify-email"

//go:embed verify_email.html
var fs embed.FS

func (processor *RedisTaskProcessor) SendVerifyEmail(pl any) error {
	// Check if the payload type is correct
	payload, ok := pl.(SendVerifyEmailPayload)
	if !ok {
		return fmt.Errorf("invalid payload type for this task")
	}

	// Generate OTP
	otp := security.GenerateRandomOTP()
	payload.OTP = otp

	// Check if the OTP already registered to avoid collisions
	ok = false
	for !ok {
		res, err := processor.queries.GetCache(context.Background(), otp)
		if err != nil && err.Error() != "cache miss" {
			return fmt.Errorf("failed to check if OTP exists in cache: %v", err)
		}

		// If cache miss
		if res != "" {
			otp = security.GenerateRandomOTP()
		} else {
			ok = true
		}
	}

	// Prepare the HTML email body
	tmpl, err := template.ParseFS(fs, "verify_email.html")
	if err != nil {
		return err
	}
	var buffer bytes.Buffer
	if err = tmpl.Execute(&buffer, payload); err != nil {
		return err
	}

	// Send email
	err = processor.mailService.SendEmail(payload.Email, "Welcome to Ticket - Verify your account", buffer.String())
	if err != nil {
		return err
	}

	// Register OTP into cache
	processor.queries.SetCache(context.Background(), payload.ID.String(), otp, time.Second*60)

	return nil
}
