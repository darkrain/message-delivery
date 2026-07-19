package fake

import (
	"context"
	"sync"

	"github.com/darkrain/message-delivery/internal/provider"
)

type Provider struct {
	name   string
	status string
	mu     sync.Mutex
	Sent   []provider.Message
}

func New(name string, status string) *Provider {
	if status == "" {
		status = provider.StatusSent
	}
	return &Provider{name: name, status: status}
}

func (p *Provider) Name() string {
	return p.name
}

func (p *Provider) Send(_ context.Context, msg provider.Message) provider.Result {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Sent = append(p.Sent, msg)
	switch p.status {
	case provider.StatusSent:
		return provider.Result{Status: provider.StatusSent}
	case provider.StatusUndeliverable:
		return provider.Result{Status: provider.StatusUndeliverable, ErrorCode: "provider_not_available"}
	default:
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "provider_failed"}
	}
}

func (p *Provider) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.Sent)
}
