package worker

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"tekticket/util"
	"time"
)

type SendVerifyEmailPayload struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	OTP      string `json:"otp"`
}

const SendVerifyEmail = "send-verify-email"

//go:embed verify_email.html
var verifyFS embed.FS

func (processor *RedisTaskProcessor) SendVerifyEmail(payload SendVerifyEmailPayload) error {
	// Generate OTP
	otp := util.GenerateRandomOTP()
	payload.OTP = otp

	// Check if the OTP already registered to avoid collisions
	ok := false
	for !ok {
		_, err := processor.queries.GetCache(context.Background(), otp)
		if processor.queries.IsCacheMiss(err) {
			// If cache miss
			ok = true
		} else {
			// If cache error, return error
			if err != nil {
				return fmt.Errorf("failed to check if OTP exists in cache: %v", err)
			}

			// If cache hit -> OTP in used -> create new one
			otp = util.GenerateRandomOTP()
		}
	}

	// Prepare the HTML email body
	tmpl, err := template.ParseFS(verifyFS, "verify_email.html")
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
	// Although the OTP should expired in 30 seconds, we add some time for latency
	processor.queries.SetCache(context.Background(), otp, payload.ID, time.Second*45)

	return nil
}
