package app

import (
	"context"
	"fmt"
	"time"

	"github.com/la5nta/pat/api/types"
	"github.com/la5nta/pat/internal/debug"
)

const (
	PromptKindBusyChannel = types.PromptKindBusyChannel
	PromptKindMultiSelect = types.PromptKindMultiSelect
	PromptKindPassword    = types.PromptKindPassword
)

type (
	PromptResponse = types.PromptResponse
	PromptKind     = types.PromptKind
	PromptOption   = types.PromptOption
)

type Prompt struct {
	types.Prompt

	hub    *PromptHub
	resp   chan PromptResponse
	ctx    context.Context
	cancel context.CancelFunc
}

type Prompter interface{ Prompt(Prompt) }

func (p Prompt) Done() <-chan struct{}         { return p.ctx.Done() }
func (p Prompt) Err() error                    { return p.ctx.Err() }
func (p Prompt) Respond(val string, err error) { p.hub.Respond(p.ID, val, err) }

type PromptHub struct {
	c  chan *Prompt
	rc chan PromptResponse

	prompters map[Prompter]struct{}
}

func NewPromptHub() *PromptHub {
	p := &PromptHub{}
	go p.loop()
	return p
}

func (p *PromptHub) AddPrompter(prompters ...Prompter) {
	if p.prompters == nil {
		p.prompters = make(map[Prompter]struct{}, len(prompters))
	}
	for _, prompter := range prompters {
		p.prompters[prompter] = struct{}{}
	}
}

func (p *PromptHub) loop() {
	p.c = make(chan *Prompt)
	p.rc = make(chan PromptResponse, 1)
	for prompt := range p.c {
		debug.Printf("New prompt: %#v", prompt)
		select {
		case <-prompt.ctx.Done():
			debug.Printf("Prompt cancelled: %v", prompt.ctx.Err())
			prompt.resp <- PromptResponse{ID: prompt.ID, Err: prompt.ctx.Err()}
		case resp := <-p.rc:
			debug.Printf("Prompt resp: %#v", resp)
			if resp.ID != prompt.ID {
				continue
			}
			prompt.resp <- resp
			prompt.cancel()
		}
	}
}

func (p *PromptHub) Respond(id, value string, err error) {
	select {
	case p.rc <- PromptResponse{ID: id, Value: value, Err: err}:
	default:
	}
}

func (p *PromptHub) Prompt(ctx context.Context, timeout time.Duration, kind PromptKind, message string, options ...PromptOption) <-chan PromptResponse {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	prompt := &Prompt{
		Prompt: types.Prompt{
			ID:      fmt.Sprint(time.Now().UnixNano()),
			Kind:    kind,
			Message: message,
			Options: options,
		},
		hub:    p,
		resp:   make(chan PromptResponse, 1),
		ctx:    ctx,
		cancel: cancel,
	}
	p.c <- prompt

	for prompter, _ := range p.prompters {
		prompter := prompter
		go prompter.Prompt(*prompt)
	}

	return prompt.resp
}
