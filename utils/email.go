package utils

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
)

// SendEmail sends a plain text email using SMTP.
// It supports STARTTLS for port 587 and SSL/TLS for port 465.
func SendEmail(host string, port int, user, password, from, to, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	
	// RFC 822 message format
	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"\r\n"+
		"%s\r\n", from, to, subject, body)

	// For port 465, we usually need direct TLS
	if port == 465 {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         host,
		}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("tls dial failed: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, host)
		if err != nil {
			return fmt.Errorf("smtp client failed: %w", err)
		}
		defer client.Quit()

		if err = client.Auth(smtp.PlainAuth("", user, password, host)); err != nil {
			return fmt.Errorf("smtp auth failed: %w", err)
		}
		if err = client.Mail(from); err != nil {
			return fmt.Errorf("smtp mail command failed: %w", err)
		}
		if err = client.Rcpt(to); err != nil {
			return fmt.Errorf("smtp rcpt command failed: %w", err)
		}
		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("smtp data command failed: %w", err)
		}
		_, err = w.Write([]byte(msg))
		if err != nil {
			return fmt.Errorf("failed to write message: %w", err)
		}
		err = w.Close()
		if err != nil {
			return fmt.Errorf("failed to close data writer: %w", err)
		}
		return nil
	}

	// For other ports (587, 25), use standard smtp package which handles STARTTLS
	auth := smtp.PlainAuth("", user, password, host)
	err := smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
	if err != nil {
		return fmt.Errorf("smtp send failed: %w", err)
	}
	
	return nil
}
