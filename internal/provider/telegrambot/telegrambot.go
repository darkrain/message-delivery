package telegrambot

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/darkrain/message-delivery/internal/config"
	"github.com/darkrain/message-delivery/internal/provider"
)

const defaultBaseURL = "https://api.telegram.org"

type Provider struct {
	name          string
	baseURL       string
	botToken      string
	publicBaseURL string
	presentation  config.TelegramBotPresentation
	httpClient    *http.Client
}

func New(name, baseURL, botToken, publicBaseURL string, timeout time.Duration, presentation config.TelegramBotPresentation) *Provider {
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
		presentation:  presentation.WithDefaults(),
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

	text, replyMarkup := telegramMessage(message, p.publicBaseURL, p.presentation)
	payload, err := json.Marshal(sendMessageRequest{
		ChatID:                chatID,
		Text:                  text,
		ParseMode:             "HTML",
		DisableWebPagePreview: true,
		LinkPreviewOptions:    &linkPreviewOptions{IsDisabled: true},
		ReplyMarkup:           replyMarkup,
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
	ParseMode             string              `json:"parse_mode,omitempty"`
	DisableWebPagePreview bool                `json:"disable_web_page_preview"`
	LinkPreviewOptions    *linkPreviewOptions `json:"link_preview_options,omitempty"`
	ReplyMarkup           *inlineKeyboard     `json:"reply_markup,omitempty"`
}

type linkPreviewOptions struct {
	IsDisabled bool `json:"is_disabled"`
}

type inlineKeyboard struct {
	InlineKeyboard [][]inlineKeyboardButton `json:"inline_keyboard"`
}

type inlineKeyboardButton struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

type botResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

func telegramMessage(message provider.Message, publicBaseURL string, presentation config.TelegramBotPresentation) (string, *inlineKeyboard) {
	presentation = presentation.WithDefaults()
	locale := telegramLocale(message.Metadata["locale"])
	if message.Metadata["telegram_presentation"] == "welcome" {
		return telegramWelcomeText(message, locale, presentation), nil
	}

	title := firstNonBlank(message.Metadata["telegram_title"], message.Subject, localized(presentation.NotificationFallbackTitle, locale))
	title = telegramNotificationTitle(title, message.Metadata["telegram_event_type"], locale)
	body := firstNonBlank(message.Metadata["telegram_body"], message.Body)
	parts := []string{telegramNotificationEmoji(message.Metadata) + " <b>" + html.EscapeString(truncateRunes(title, 320)) + "</b>"}
	if body = strings.TrimSpace(body); body != "" && body != title {
		parts = append(parts, html.EscapeString(truncateRunes(body, 3000)))
	}
	if footer := localized(presentation.NotificationFooter, locale); footer != "" {
		parts = append(parts, "<i>"+html.EscapeString(footer)+"</i>")
	}

	var replyMarkup *inlineKeyboard
	if target := telegramTargetURL(publicBaseURL, message.Metadata["telegram_target_path"]); target != "" {
		replyMarkup = &inlineKeyboard{InlineKeyboard: [][]inlineKeyboardButton{{{
			Text: localized(presentation.OpenActionLabel, locale), URL: target,
		}}}}
	}
	return strings.Join(parts, "\n\n"), replyMarkup
}

func telegramWelcomeText(message provider.Message, locale string, presentation config.TelegramBotPresentation) string {
	title := firstNonBlank(message.Subject, localized(presentation.WelcomeTitle, locale))
	body := firstNonBlank(message.Body, localized(presentation.WelcomeBody, locale))
	parts := []string{"👋 <b>" + html.EscapeString(truncateRunes(title, 320)) + "</b>"}
	if body = strings.TrimSpace(body); body != "" && body != title {
		parts = append(parts, html.EscapeString(truncateRunes(body, 3000)))
	}
	return strings.Join(parts, "\n\n")
}

func telegramNotificationTitle(title, eventType, locale string) string {
	if eventType != "chat_message_received" || title == "" {
		return title
	}
	if locale == "ru" {
		return "Новое сообщение от " + title
	}
	return "New message from " + title
}

func telegramNotificationEmoji(metadata map[string]string) string {
	if metadata["telegram_priority"] == "critical" {
		return "🚨"
	}
	switch metadata["telegram_icon"] {
	case "chat":
		return "💬"
	case "card", "cash", "money":
		return "💳"
	case "pin", "map":
		return "📍"
	case "user", "profile":
		return "👤"
	case "flag":
		return "🚩"
	case "offer":
		return "✨"
	case "shield", "lock":
		return "🛡️"
	case "calendar":
		return "📅"
	default:
		return "🔔"
	}
}

func telegramLocale(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if index := strings.IndexByte(value, '-'); index >= 0 {
		value = value[:index]
	}
	if value == "ru" {
		return "ru"
	}
	return "en"
}

func localized(values map[string]string, locale string) string {
	if value := strings.TrimSpace(values[locale]); value != "" {
		return value
	}
	return strings.TrimSpace(values["en"])
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
