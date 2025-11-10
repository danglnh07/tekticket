package worker

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"strconv"
	"strings"

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

// Helper method: generate reset password token. Since this method only use internally for the processor to send email,
// we are not export it.
func (processor *RedisTaskProcessor) generateResetPasswordToken(id, email string) (string, error) {
	// Generate token
	rawToken := fmt.Sprintf("%s#%s#%d", id, email, time.Now().UnixNano())
	encrypt, err := util.Encrypt([]byte(processor.config.SecretKey), []byte(rawToken))
	if err != nil {
		return "", err
	}
	return util.Encode(string(encrypt)), nil
}

func VerifyResetPasswordToken(token string, secretKey string) ([]string, error) {
	// Decode base64 token
	decodeToken, err := util.Decode(token)
	if err != nil {
		return nil, err
	}

	// Decrypt token
	raw, err := util.Decrypt([]byte(secretKey), []byte(decodeToken))
	if err != nil {
		return nil, err
	}

	// Split the raw token into segments, separate by the delimiter #
	segments := strings.Split(string(raw), "#")
	if len(segments) != 3 {
		return nil, fmt.Errorf("invalid token, segments length must be 3")
	}

	// Check if token has expired or not
	timestamp, err := strconv.ParseInt(segments[2], 10, 64)
	if err != nil {
		return nil, err
	}

	if time.Now().After(time.Unix(0, int64(timestamp)).Add(time.Hour)) {
		return nil, fmt.Errorf("token expired")
	}

	return segments, nil
}

// Helper method: verify reset password token. This should be use by the client (API handler), so it should be exported
func (processor *RedisTaskProcessor) SendResetPassword(payload SendResetPasswordPayload) error {
	// Generate token
	token, err := processor.generateResetPasswordToken(payload.ID, payload.Email)
	if err != nil {
		return err
	}

	// Create reset link
	link := fmt.Sprintf("%s?token=%s", processor.config.ResetPasswordURL, token)
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
