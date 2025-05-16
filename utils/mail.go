package utils

import (
	"bytes"
	"fmt"
	"html/template"
	"net/smtp"
	"os"
)

type EmailData struct {
	Name            string
	Message         string
	VerificationURL string
	LogoURL         string
}

func SendEmail(emailTo string, emailSubject string, data EmailData, templatePath string) error {

	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return fmt.Errorf("template parse error: %w", err)
	}

	var body bytes.Buffer
	err = tmpl.Execute(&body, data)
	if err != nil {
		return fmt.Errorf("template execution error: %w", err)
	}

	message := fmt.Sprintf(
		"From: %s\r\nSubject: %s\r\nMIME-version: 1.0;\r\nContent-Type: text/html; charset=\"UTF-8\";\r\n\r\n%s",
		os.Getenv("FROM_EMAIL"),
		emailSubject,
		body.String(),
	)

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
