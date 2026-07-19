package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/darkrain/message-delivery/internal/provider"
)

const defaultBaseURL = "https://gatewayapi.telegram.org"

type Gateway struct {
	name       string
	baseURL    string
	apiToken   string
	httpClient *http.Client
}

func NewGateway(name, baseURL, apiToken string, timeout time.Duration) *Gateway {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Gateway{
		name:     name,
		baseURL:  baseURL,
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (p *Gateway) Name() string {
	return p.name
}

func (p *Gateway) Send(ctx context.Context, msg provider.Message) provider.Result {
	if p.apiToken == "" {
		return provider.Result{Status: provider.StatusUndeliverable, ErrorCode: "telegram_token_missing"}
	}
	code := msg.Variables["code"]
	if !validNumericCode(code) {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "telegram_code_invalid"}
	}

	payload := map[string]any{
		"phone_number": msg.Recipient,
		"code":         code,
		"payload":      msg.EventID,
	}
	if ttlSec, err := strconv.Atoi(msg.Variables["ttl_sec"]); err == nil && ttlSec > 0 {
		payload["ttl"] = ttlSec
	}
	if sender := msg.Metadata["telegram_sender_username"]; sender != "" {
		payload["sender_username"] = sender
	}
	if callbackURL := msg.Metadata["telegram_callback_url"]; callbackURL != "" {
		payload["callback_url"] = callbackURL
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "telegram_payload_invalid"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/sendVerificationMessage", bytes.NewReader(body))
	if err != nil {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "telegram_request_invalid"}
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return provider.Result{Status: provider.StatusUndeliverable, ErrorCode: "telegram_unavailable"}
	}
	defer resp.Body.Close()

	var envelope struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&envelope); err != nil {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "telegram_bad_response"}
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "telegram_auth_failed"}
	}
	if resp.StatusCode >= 500 {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "telegram_server_error"}
	}
	if !envelope.OK {
		return provider.Result{Status: provider.StatusUndeliverable, ErrorCode: fmt.Sprintf("telegram_%s", normalizeError(envelope.Error))}
	}
	return provider.Result{Status: provider.StatusSent}
}

func validNumericCode(code string) bool {
	if len(code) < 4 || len(code) > 8 {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func normalizeError(value string) string {
	if value == "" {
		return "undeliverable"
	}
	out := make([]rune, 0, len(value))
	for _, r := range value {
		if r >= 'A' && r <= 'Z' {
			r = r - 'A' + 'a'
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out = append(out, r)
			continue
		}
		out = append(out, '_')
	}
	return string(out)
}
