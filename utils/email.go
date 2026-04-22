package utils

import (
	"fmt"
	"net/smtp"
)

// SendEmail sends a plain text email using SMTP.
func SendEmail(host string, port int, user, password, from, to, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	
	// RFC 822 message format
	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"\r\n"+
		"%s\r\n", from, to, subject, body)

	auth := smtp.PlainAuth("", user, password, host)
	
	err := smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	
	return nil
}
