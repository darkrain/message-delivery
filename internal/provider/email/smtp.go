package email

import (
	"context"
	"net/smtp"
	"strconv"
	"strings"

	"github.com/darkrain/message-delivery/internal/provider"
)

type SMTP struct {
	name     string
	host     string
	port     int
	username string
	password string
	from     string
}

func NewSMTP(name, host string, port int, username, password, from string) *SMTP {
	return &SMTP{
		name:     name,
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
	}
}

func (p *SMTP) Name() string {
	return p.name
}

func (p *SMTP) Send(ctx context.Context, msg provider.Message) provider.Result {
	if p.host == "" || p.port <= 0 || p.from == "" {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "smtp_not_configured"}
	}
	select {
	case <-ctx.Done():
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "smtp_context_cancelled"}
	default:
	}

	addr := p.host + ":" + strconv.Itoa(p.port)
	auth := smtp.Auth(nil)
	if p.username != "" || p.password != "" {
		auth = smtp.PlainAuth("", p.username, p.password, p.host)
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

	if err := smtp.SendMail(addr, auth, p.from, []string{msg.Recipient}, []byte(payload)); err != nil {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "smtp_send_failed"}
	}
	return provider.Result{Status: provider.StatusSent}
}
