package worker

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"

	"tekticket/util"
	"time"
)

type SendResetPasswordPayload struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	ResetLink string `json:"reset_link"`
}

const SendResetPassword = "send-reset-password"

//go:embed reset_password.html
var resetFS embed.FS

func (processor *RedisTaskProcessor) SendResetPassword(payload SendResetPasswordPayload) error {
	// Generate token
	rawToken := fmt.Sprintf("%s#%s#%d", payload.ID, payload.Email, time.Now().UnixNano())
	encrypt, err := util.Encrypt([]byte(processor.config.SecretKey), []byte(rawToken))
	if err != nil {
		return err
	}
	token := util.Encode(string(encrypt))

	// Create reset link
	link := fmt.Sprintf("%s?token=%s", processor.config.FrontendURL, token)
	payload.ResetLink = link
	util.LOGGER.Info("Link", "val", link)

	// Prepare the HTML email body
	tmpl, err := template.ParseFS(resetFS, "reset_password.html")
	if err != nil {
		return err
	}
	var buffer bytes.Buffer
	if err = tmpl.Execute(&buffer, payload); err != nil {
		return err
	}

	// Send email
	err = processor.mailService.SendEmail(payload.Email, "Reset your password", buffer.String())
	if err != nil {
		return err
	}

	return nil
}
