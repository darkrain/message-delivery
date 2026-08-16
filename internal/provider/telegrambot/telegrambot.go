package telegrambot

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/darkrain/message-delivery/internal/provider"
)

const defaultBaseURL = "https://api.telegram.org"

type Provider struct {
	name          string
	baseURL       string
	botToken      string
	publicBaseURL string
	httpClient    *http.Client
}

func New(name, baseURL, botToken, publicBaseURL string, timeout time.Duration) *Provider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Provider{
		name:          name,
		baseURL:       strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		botToken:      strings.TrimSpace(botToken),
		publicBaseURL: strings.TrimRight(strings.TrimSpace(publicBaseURL), "/"),
		httpClient:    &http.Client{Timeout: timeout},
	}
}

func (p *Provider) Name() string { return p.name }

func (p *Provider) Send(ctx context.Context, message provider.Message) provider.Result {
	if p.botToken == "" {
		return provider.Result{Status: provider.StatusUndeliverable, ErrorCode: "telegram_bot_not_configured"}
	}
	chatID, err := strconv.ParseInt(strings.TrimSpace(message.Recipient), 10, 64)
	if err != nil || chatID <= 0 {
		return provider.Result{Status: provider.StatusUndeliverable, ErrorCode: "telegram_bot_chat_invalid"}
	}

	payload, err := json.Marshal(sendMessageRequest{
		ChatID:                chatID,
		Text:                  telegramText(message, p.publicBaseURL),
		DisableWebPagePreview: true,
		LinkPreviewOptions:    &linkPreviewOptions{IsDisabled: true},
	})
	if err != nil {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "telegram_bot_payload_invalid"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/bot"+p.botToken+"/sendMessage", bytes.NewReader(payload))
	if err != nil {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "telegram_bot_request_invalid"}
	}
	req.Header.Set("Content-Type", "application/json")

	response, err := p.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return provider.Result{Status: provider.StatusFailed, ErrorCode: "telegram_bot_timeout"}
		}
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "telegram_bot_unavailable"}
	}
	defer response.Body.Close()

	var envelope botResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&envelope); err != nil {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "telegram_bot_bad_response"}
	}
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices && envelope.OK {
		return provider.Result{Status: provider.StatusSent}
	}
	if response.StatusCode == http.StatusUnauthorized {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "telegram_bot_auth_failed"}
	}
	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "telegram_bot_service_unavailable"}
	}
	if response.StatusCode == http.StatusForbidden || telegramChatUnavailable(envelope.Description) {
		return provider.Result{Status: provider.StatusUndeliverable, ErrorCode: "telegram_bot_chat_unavailable"}
	}
	return provider.Result{Status: provider.StatusFailed, ErrorCode: "telegram_bot_rejected"}
}

type sendMessageRequest struct {
	ChatID                int64               `json:"chat_id"`
	Text                  string              `json:"text"`
	DisableWebPagePreview bool                `json:"disable_web_page_preview"`
	LinkPreviewOptions    *linkPreviewOptions `json:"link_preview_options,omitempty"`
}

type linkPreviewOptions struct {
	IsDisabled bool `json:"is_disabled"`
}

type botResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

func telegramText(message provider.Message, publicBaseURL string) string {
	title := firstNonBlank(message.Metadata["telegram_title"], message.Subject)
	body := firstNonBlank(message.Metadata["telegram_body"], message.Body)
	parts := make([]string, 0, 3)
	if title != "" {
		parts = append(parts, title)
	}
	if body != "" && body != title {
		parts = append(parts, body)
	}
	if target := telegramTargetURL(publicBaseURL, message.Metadata["telegram_target_path"]); target != "" {
		parts = append(parts, target)
	}
	return truncateRunes(strings.Join(parts, "\n\n"), 4096)
}

func telegramTargetURL(publicBaseURL, targetPath string) string {
	if publicBaseURL == "" || !strings.HasPrefix(targetPath, "/") {
		return ""
	}
	base, err := url.Parse(publicBaseURL)
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return ""
	}
	return strings.TrimRight(publicBaseURL, "/") + targetPath
}

func telegramChatUnavailable(description string) bool {
	description = strings.ToLower(description)
	return strings.Contains(description, "chat not found") || strings.Contains(description, "bot was blocked") || strings.Contains(description, "user is deactivated") || strings.Contains(description, "bot was kicked")
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	if limit <= 3 {
		return string([]rune(value)[:limit])
	}
	return string([]rune(value)[:limit-3]) + "..."
}

var _ provider.Provider = (*Provider)(nil)
