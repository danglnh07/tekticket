package mail

import (
	"fmt"
	"net/smtp"
	"strings"
)

// Universal interface for mail service
type MailService interface {
	SendEmail(to, subject, body string) error
}

// Email service struct, which holds configurations related to email sending
type EmailService struct {
	Host  string
	Port  string
	Email string
	Auth  smtp.Auth
}

// Constructing method for email service struct
func NewEmailService(email, password string) *EmailService {
	// Try simple authentication
	smtpAuth := smtp.PlainAuth("", email, password, "smtp.gmail.com")

	return &EmailService{
		Host:  "smtp.gmail.com",
		Port:  "587",
		Email: email,
		Auth:  smtpAuth,
	}
}

// Method to send email
func (service *EmailService) SendEmail(to, subject, body string) error {
	// Set email headers with MIME version and content type
	headers := make(map[string]string)
	headers["From"] = service.Email
	headers["To"] = to
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	// Build the message with headers
	var message strings.Builder
	for key, value := range headers {
		message.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
	}
	message.WriteString("\r\n")
	message.WriteString(body)

	addr := fmt.Sprintf("%s:%s", service.Host, service.Port)
	return smtp.SendMail(
		addr,
		service.Auth,
		service.Email,
		[]string{to},
		[]byte(message.String()),
	)
}
