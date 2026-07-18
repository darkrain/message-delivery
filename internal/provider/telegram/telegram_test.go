package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/darkrain/message-delivery/internal/provider"
)

func TestGatewaySendVerificationMessage(t *testing.T) {
	var capturedPath string
	var capturedAuth string
	var capturedPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&capturedPayload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"request_id":"req-1"}}`))
	}))
	defer server.Close()

	gateway := NewGateway("telegram", server.URL, "token-1")
	result := gateway.Send(context.Background(), provider.Message{
		EventID:   "event-1",
		Recipient: "+10000000000",
		Variables: map[string]string{
			"code":    "123456",
			"ttl_sec": "300",
		},
	})

	if result.Status != provider.StatusSent {
		t.Fatalf("result = %#v", result)
	}
	if capturedPath != "/sendVerificationMessage" {
		t.Fatalf("path = %q", capturedPath)
	}
	if capturedAuth != "Bearer token-1" {
		t.Fatalf("auth header = %q", capturedAuth)
	}
	if capturedPayload["phone_number"] != "+10000000000" || capturedPayload["code"] != "123456" {
		t.Fatalf("payload = %#v", capturedPayload)
	}
}

func TestGatewayMissingTokenAllowsFallback(t *testing.T) {
	gateway := NewGateway("telegram", "", "")
	result := gateway.Send(context.Background(), provider.Message{
		Recipient: "+10000000000",
		Variables: map[string]string{"code": "123456"},
	})

	if result.Status != provider.StatusUndeliverable || result.ErrorCode != "telegram_token_missing" {
		t.Fatalf("result = %#v", result)
	}
}

func TestGatewaySendVerificationMessageLive(t *testing.T) {
	if os.Getenv("TELEGRAM_GATEWAY_LIVE_TEST") != "1" {
		t.Skip("set TELEGRAM_GATEWAY_LIVE_TEST=1 to run Telegram Gateway live test")
	}
	token := os.Getenv("TELEGRAM_GATEWAY_API_TOKEN")
	if token == "" {
		t.Skip("set TELEGRAM_GATEWAY_API_TOKEN to run Telegram Gateway live test")
	}
	phone := os.Getenv("TELEGRAM_GATEWAY_TEST_PHONE")
	if phone == "" {
		t.Skip("set TELEGRAM_GATEWAY_TEST_PHONE in E.164 format to run Telegram Gateway live test")
	}

	code := time.Now().UTC().Format("040506")
	gateway := NewGateway("telegram", "", token)
	result := gateway.Send(context.Background(), provider.Message{
		EventID:   "live-test-" + time.Now().UTC().Format("20060102150405"),
		Recipient: phone,
		Variables: map[string]string{
			"code":    code,
			"ttl_sec": "60",
		},
	})

	if result.Status != provider.StatusSent {
		t.Fatalf("result = %#v", result)
	}
}
