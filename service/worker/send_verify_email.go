package worker

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"tekticket/db"
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
	var cacheMiss *db.ErrorCacheMiss
	for !ok {
		res, err := processor.queries.GetCache(context.Background(), otp)
		if err != nil && !errors.As(err, &cacheMiss) {
			return fmt.Errorf("failed to check if OTP exists in cache: %v", err)
		}

		// Check if cache miss
		if res != "" {
			otp = util.GenerateRandomOTP()
		} else {
			ok = true
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
