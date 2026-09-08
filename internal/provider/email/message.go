package email

import (
	"fmt"
	"html"
	"net/url"
	"strings"

	"github.com/darkrain/message-delivery/internal/provider"
	"gopkg.in/gomail.v2"
)

// Only explicit notification metadata enables unsubscribe. Transactional
// authentication mail has no such metadata and remains unchanged.
func buildMessage(from string, msg provider.Message) (*gomail.Message, error) {
	mail := gomail.NewMessage()
	mail.SetHeader("From", from)
	mail.SetHeader("To", msg.Recipient)
	mail.SetHeader("Subject", msg.Subject)
	contentType := msg.ContentType
	if contentType == "" {
		contentType = "text/plain; charset=UTF-8"
	}
	body := msg.Body
	if address := msg.Metadata["email_unsubscribe_url"]; address != "" {
		u, err := url.Parse(address)
		if err != nil || len(address) > 2048 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || strings.ContainsAny(address, "\r\n\t <>\"") {
			return nil, fmt.Errorf("invalid unsubscribe URL")
		}
		mail.SetHeader("List-Unsubscribe", "<"+address+">")
		mail.SetHeader("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
		if msg.Metadata["email_unsubscribe_in_body"] != "true" {
			label := msg.Metadata["email_unsubscribe_label"]
			if label == "" {
				return nil, fmt.Errorf("missing unsubscribe label")
			}
			if strings.HasPrefix(strings.ToLower(contentType), "text/html") {
				body += `<p><a href="` + html.EscapeString(address) + `">` + html.EscapeString(label) + `</a></p>`
			} else {
				body += "\n\n" + label + ": " + address
			}
		}
	}
	mail.SetBody(contentType, body)
	return mail, nil
}
