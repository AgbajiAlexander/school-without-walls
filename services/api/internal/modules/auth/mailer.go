package auth

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"strings"
	"time"
)

type Mailer interface {
	SendPasswordReset(ctx context.Context, email, token string, expiresAt time.Time) error
}

type SMTPMailer struct {
	host         string
	port         int
	username     string
	password     string
	from         string
	publicWebURL string
}

func NewSMTPMailer(host string, port int, username, password, from, publicWebURL string) *SMTPMailer {
	return &SMTPMailer{
		host:         host,
		port:         port,
		username:     username,
		password:     password,
		from:         from,
		publicWebURL: strings.TrimRight(publicWebURL, "/"),
	}
}

func (m *SMTPMailer) SendPasswordReset(ctx context.Context, email, token string, expiresAt time.Time) error {
	to, err := mail.ParseAddress(email)
	if err != nil || to.Address != email {
		return fmt.Errorf("invalid password-reset recipient")
	}
	from, err := mail.ParseAddress(m.from)
	if err != nil {
		return fmt.Errorf("invalid configured SMTP sender")
	}
	resetURL, err := url.Parse(m.publicWebURL + "/reset-password")
	if err != nil {
		return fmt.Errorf("invalid public web URL")
	}
	query := resetURL.Query()
	query.Set("token", token)
	resetURL.RawQuery = query.Encode()

	message := "From: " + from.String() + "\r\n" +
		"To: " + to.String() + "\r\n" +
		"Subject: Reset your School Without Walls password\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		"Use this one-time link to reset your password:\r\n" + resetURL.String() +
		"\r\n\r\nThis link expires at " + expiresAt.UTC().Format(time.RFC3339) +
		". If you did not request this, you can ignore this message.\r\n"

	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(m.host, fmt.Sprint(m.port)))
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server")
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		return fmt.Errorf("failed to set SMTP deadline")
	}
	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return fmt.Errorf("failed to initialize SMTP client")
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); !ok {
		return fmt.Errorf("SMTP server does not support STARTTLS")
	}
	if err := client.StartTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: m.host}); err != nil {
		return fmt.Errorf("failed to establish SMTP TLS")
	}
	if m.username != "" {
		if err := client.Auth(smtp.PlainAuth("", m.username, m.password, m.host)); err != nil {
			return fmt.Errorf("SMTP authentication failed")
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("SMTP sender was rejected")
	}
	if err := client.Rcpt(to.Address); err != nil {
		return fmt.Errorf("SMTP recipient was rejected")
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP message was rejected")
	}
	if _, err := writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("failed to write SMTP message")
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to send SMTP message")
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("failed to close SMTP session")
	}
	return nil
}
