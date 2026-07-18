package email

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/darkrain/message-delivery/internal/provider"
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
	addr := p.host + ":" + strconv.Itoa(p.port)
	client, err := p.connect(ctx, addr)
	if err != nil {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "smtp_connect_failed"}
	}
	defer client.Close()

	if p.username != "" || p.password != "" {
		if err := client.Auth(smtp.PlainAuth("", p.username, p.password, p.authHost)); err != nil {
			return provider.Result{Status: provider.StatusFailed, ErrorCode: "smtp_auth_failed"}
		}
	}
	payload := strings.Join([]string{
		"From: " + p.from,
		"To: " + msg.Recipient,
		"Subject: " + msg.Subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		msg.Body,
	}, "\r\n")

	if err := client.Mail(p.from); err != nil {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "smtp_mail_failed"}
	}
	if err := client.Rcpt(msg.Recipient); err != nil {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "smtp_rcpt_failed"}
	}
	writer, err := client.Data()
	if err != nil {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "smtp_data_failed"}
	}
	if _, err := writer.Write([]byte(payload)); err != nil {
		_ = writer.Close()
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "smtp_write_failed"}
	}
	if err := writer.Close(); err != nil {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "smtp_send_failed"}
	}
	_ = client.Quit()
	return provider.Result{Status: provider.StatusSent}
}

func (p *SMTP) connect(ctx context.Context, addr string) (*smtp.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	dialer := &net.Dialer{Timeout: p.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Now().Add(p.timeout))

	if p.security == "tls" || (p.security == "" && p.port == 465) {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: p.authHost, MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, err
		}
		return smtp.NewClient(tlsConn, p.authHost)
	}

	client, err := smtp.NewClient(conn, p.authHost)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if p.security == "starttls" || p.security == "" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: p.authHost, MinVersion: tls.VersionTLS12}); err != nil {
				_ = client.Close()
				return nil, err
			}
		} else if p.security == "starttls" {
			_ = client.Close()
			return nil, errors.New("smtp: starttls is not supported")
		}
	}
	return client, nil
}
