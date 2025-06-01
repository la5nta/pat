package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/howeyc/gopass"
	"github.com/la5nta/pat/internal/debug"
)

type PromptKind string

const (
	PromptKindPassword    PromptKind = "password"
	PromptKindMultiSelect PromptKind = "multi-select"
	PromptKindBusyChannel PromptKind = "busy-channel"
)

type Prompt struct {
	ID      string         `json:"id"`
	Kind    PromptKind     `json:"kind"`
	Message string         `json:"message"`
	Options []PromptOption `json:"options,omitempty"` // For multi-select

	resp   chan PromptResponse
	ctx    context.Context
	cancel context.CancelFunc
}

type PromptOption struct {
	Value   string `json:"value"`
	Desc    string `json:"desc,omitempty"`
	Checked bool   `json:"checked"`
}

type PromptResponse struct {
	ID    string `json:"id"`
	Value string `json:"value"`
	Err   error  `json:"error"`
}

type PromptHub struct {
	c  chan *Prompt
	rc chan PromptResponse

	omitTerminal bool
}

func NewPromptHub() *PromptHub { p := new(PromptHub); go p.loop(); return p }

func (p *PromptHub) OmitTerminal(t bool) { p.omitTerminal = t }

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

func (p *PromptHub) Prompt(ctx context.Context, kind PromptKind, message string, options ...PromptOption) <-chan PromptResponse {
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(time.Minute))
	prompt := &Prompt{
		resp:    make(chan PromptResponse, 1),
		ctx:     ctx,
		cancel:  cancel,
		ID:      fmt.Sprint(time.Now().UnixNano()),
		Kind:    kind,
		Message: message,
		Options: options,
	}
	p.c <- prompt

	websocketHub.Prompt(*prompt)
	if !p.omitTerminal {
		go p.promptTerminal(*prompt)
	}

	return prompt.resp
}

func (p *PromptHub) promptTerminal(prompt Prompt) {
	q := make(chan struct{}, 1)
	defer close(q)
	go func() {
		select {
		case <-prompt.ctx.Done():
			fmt.Printf(" Prompt Aborted - Press ENTER to continue...")
		case <-q:
			return
		}
	}()

	switch prompt.Kind {
	case PromptKindMultiSelect:
		fmt.Println(prompt.Message + ":")
		answers := map[string]PromptOption{}
		for idx, opt := range prompt.Options {
			answers[strconv.Itoa(idx+1)] = opt
			answers[opt.Value] = opt
			fmt.Printf("  %d: %s (%s)\n", idx+1, opt.Desc, opt.Value)
		}

		fmt.Printf("Select [1-%d, ...]: ", len(prompt.Options))
		ans := strings.FieldsFunc(readLine(), SplitFunc)
		var selected []string
		for _, str := range ans {
			opt, ok := answers[str]
			if !ok {
				log.Printf("Skipping unknown option %q", str)
				continue
			}
			selected = append(selected, opt.Value)
		}
		p.Respond(prompt.ID, strings.Join(selected, ","), nil)
	case PromptKindPassword:
		passwd, err := gopass.GetPasswdPrompt(prompt.Message+": ", true, os.Stdin, os.Stdout)
		p.Respond(prompt.ID, string(passwd), err)
	case PromptKindBusyChannel:
		fmt.Println(prompt.Message + ":")
		for prompt.ctx.Err() == nil {
			fmt.Printf("Answer [c(ontinue), a(bort)]: ")
			switch ans := readLine(); strings.TrimSpace(ans) {
			case "c", "continue":
				p.Respond(prompt.ID, "continue", nil)
				return
			case "a", "abort":
				p.Respond(prompt.ID, "abort", nil)
				return
			}
		}
	default:
		log.Printf("Prompt kind %q not implemented", prompt.Kind)
	}
}
