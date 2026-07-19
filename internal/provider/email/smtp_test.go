package email

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/darkrain/message-delivery/internal/provider"
)

func TestSMTPSendLive(t *testing.T) {
	if os.Getenv("SMTP_LIVE_TEST") != "1" {
		t.Skip("set SMTP_LIVE_TEST=1 to run SMTP live test")
	}
	host := envOrDefault("SMTP_HOST", "smtp.yandex.com")
	port := intEnvOrDefault("SMTP_PORT", 465)
	security := envOrDefault("SMTP_SECURITY", "tls")
	authHost := envOrDefault("SMTP_AUTH_HOST", host)
	username := os.Getenv("SMTP_USERNAME")
	password := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SMTP_FROM")
	to := os.Getenv("SMTP_TO")
	if username == "" || password == "" || from == "" || to == "" {
		t.Skip("set SMTP_USERNAME, SMTP_PASSWORD, SMTP_FROM and SMTP_TO to run SMTP live test")
	}

	smtpProvider := NewSMTP("smtp", host, port, authHost, username, password, from, security, 20*time.Second)
	result := smtpProvider.Send(context.Background(), provider.Message{
		Subject:   "message-delivery SMTP live test",
		Body:      "SMTP live test code: 112233",
		Recipient: to,
		Variables: map[string]string{"code": "112233", "ttl_sec": "300"},
	})
	if result.Status != provider.StatusSent {
		t.Fatalf("result = %#v", result)
	}
}

func TestSMTPErrorCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "auth", err: errors.New("535 authentication failed"), want: "smtp_auth_failed"},
		{name: "recipient", err: errors.New("550 recipient rejected"), want: "smtp_rcpt_failed"},
		{name: "timeout", err: errors.New("i/o timeout"), want: "smtp_timeout"},
		{name: "connect", err: errors.New("dial tcp: connect: connection refused"), want: "smtp_connect_failed"},
		{name: "fallback", err: errors.New("unexpected smtp error"), want: "smtp_send_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := smtpErrorCode(tt.err); got != tt.want {
				t.Fatalf("smtpErrorCode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func intEnvOrDefault(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		n, err := strconv.Atoi(value)
		if err == nil {
			return n
		}
	}
	return fallback
}
