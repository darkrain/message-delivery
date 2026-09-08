package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"github.com/darkrain/message-delivery/internal/provider"
	"gopkg.in/gomail.v2"
)

type SMTP struct {
	name       string
	host       string
	port       int
	authHost   string
	username   string
	password   string
	from       string
	security   string
	authMethod string
	htmlLayout string
	timeout    time.Duration
}

func NewSMTP(name, host string, port int, authHost, username, password, from, security string, timeout time.Duration, authMethods ...string) *SMTP {
	authMethod := "auto"
	if len(authMethods) > 0 && authMethods[0] != "" {
		authMethod = strings.ToLower(authMethods[0])
	}
	if authHost == "" {
		authHost = host
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &SMTP{
		name:       name,
		host:       host,
		port:       port,
		authHost:   authHost,
		username:   username,
		password:   password,
		from:       from,
		security:   strings.ToLower(security),
		authMethod: authMethod,
		timeout:    timeout,
	}
}

func (p *SMTP) Name() string {
	return p.name
}

func (p *SMTP) Send(ctx context.Context, msg provider.Message) provider.Result {
	if p.host == "" || p.port <= 0 || p.from == "" {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "smtp_not_configured"}
	}
	if p.authMethod != "auto" && p.authMethod != "plain" {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "smtp_auth_method_invalid"}
	}

	mail, err := buildMessage(p.from, msg, p.htmlLayout)
	if err != nil {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "smtp_invalid_message"}
	}

	dialer := p.newDialer()

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

func (p *SMTP) newDialer() *gomail.Dialer {
	dialer := gomail.NewDialer(p.host, p.port, p.username, p.password)
	dialer.SSL = p.security == "tls" || (p.security == "" && p.port == 465)
	dialer.TLSConfig = &tls.Config{ServerName: p.authHost, MinVersion: tls.VersionTLS12}

	// Explicit PLAIN avoids gomail preferring a broken advertised CRAM-MD5.
	// Auth binds to the TCP endpoint; AuthHost remains the verified TLS name.
	if p.authMethod == "plain" {
		dialer.Auth = tlsOnlyPlainAuth{smtp.PlainAuth("", p.username, p.password, p.host)}
	}
	return dialer
}

type tlsOnlyPlainAuth struct{ smtp.Auth }

func (auth tlsOnlyPlainAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS {
		return "", nil, fmt.Errorf("SMTP PLAIN authentication requires TLS")
	}
	return auth.Auth.Start(server)
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
