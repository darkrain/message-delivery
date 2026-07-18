package email

import (
	"context"
	"crypto/tls"
	"net/smtp"
	"strings"
	"time"

	"github.com/darkrain/message-delivery/internal/provider"
	"gopkg.in/gomail.v2"
)

type SMTP struct {
	name     string
	host     string
	port     int
	authHost string
	username string
	password string
	from     string
	security string
	timeout  time.Duration
}

func NewSMTP(name, host string, port int, authHost, username, password, from, security string, timeout time.Duration) *SMTP {
	if authHost == "" {
		authHost = host
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &SMTP{
		name:     name,
		host:     host,
		port:     port,
		authHost: authHost,
		username: username,
		password: password,
		from:     from,
		security: strings.ToLower(security),
		timeout:  timeout,
	}
}

func (p *SMTP) Name() string {
	return p.name
}

func (p *SMTP) Send(ctx context.Context, msg provider.Message) provider.Result {
	if p.host == "" || p.port <= 0 || p.from == "" {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "smtp_not_configured"}
	}

	mail := gomail.NewMessage()
	mail.SetHeader("From", p.from)
	mail.SetHeader("To", msg.Recipient)
	mail.SetHeader("Subject", msg.Subject)
	mail.SetBody("text/plain; charset=UTF-8", msg.Body)

	dialer := gomail.NewDialer(p.host, p.port, p.username, p.password)
	dialer.SSL = p.security == "tls" || (p.security == "" && p.port == 465)
	dialer.TLSConfig = &tls.Config{ServerName: p.authHost, MinVersion: tls.VersionTLS12}
	if p.authHost != "" && p.authHost != p.host && p.username != "" {
		dialer.Auth = smtp.PlainAuth("", p.username, p.password, p.authHost)
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- dialer.DialAndSend(mail)
	}()

	select {
	case <-ctx.Done():
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "smtp_timeout"}
	case err := <-errCh:
		if err != nil {
			return provider.Result{Status: provider.StatusFailed, ErrorCode: smtpErrorCode(err)}
		}
		return provider.Result{Status: provider.StatusSent}
	}
}

func smtpErrorCode(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "authentication") || strings.Contains(message, "auth"):
		return "smtp_auth_failed"
	case strings.Contains(message, "recipient") || strings.Contains(message, "rcpt"):
		return "smtp_rcpt_failed"
	case strings.Contains(message, "timeout") || strings.Contains(message, "deadline"):
		return "smtp_timeout"
	case strings.Contains(message, "dial") || strings.Contains(message, "connect") || strings.Contains(message, "connection"):
		return "smtp_connect_failed"
	default:
		return "smtp_send_failed"
	}
}
