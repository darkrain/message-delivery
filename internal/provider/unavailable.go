package provider

import "context"

type Unavailable struct {
	name      string
	errorCode string
}

func NewUnavailable(name, errorCode string) *Unavailable {
	return &Unavailable{name: name, errorCode: errorCode}
}

func (p *Unavailable) Name() string {
	return p.name
}

func (p *Unavailable) Send(_ context.Context, _ Message) Result {
	return Result{Status: StatusUndeliverable, ErrorCode: p.errorCode}
}
