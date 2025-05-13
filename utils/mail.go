package utils

import (
	"bytes"
	"fmt"
	"html/template"
	"net/smtp"
	"os"
	"path/filepath"
)

type EmailData struct {
	Name            string
	Message         string
	VerificationURL string
	LogoURL         string
}

func SendEmail(emailTo string, emailSubject string, data EmailData) error {
	templatePath := filepath.Join("templates", "verify_email.html")

	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return fmt.Errorf("template parse error: %w", err)
	}

	var body bytes.Buffer
	err = tmpl.Execute(&body, data)
	if err != nil {
		return fmt.Errorf("template execution error: %w", err)
	}

	message := fmt.Sprintf("Subject: %s\r\n", emailSubject) +
		"MIME-version: 1.0;\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\";\r\n\r\n" +
		body.String()

	auth := smtp.PlainAuth(
		"",
		os.Getenv("FROM_EMAIL"),
		os.Getenv("FROM_EMAIL_PASSWORD"),
		os.Getenv("FROM_EMAIL_SMTP"),
	)

	err = smtp.SendMail(os.Getenv("SMTP_ADDRESS"), auth, os.Getenv("FROM_EMAIL"), []string{emailTo}, []byte(message))
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}
