package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/howeyc/gopass"
)

type PromptKind string

const (
	PromptKindPassword    PromptKind = "password"
	PromptKindMultiSelect PromptKind = "multi-select"
)

type Prompt struct {
	resp     chan PromptResponse
	cancel   chan struct{}
	ID       string         `json:"id"`
	Kind     PromptKind     `json:"kind"`
	Deadline time.Time      `json:"deadline"`
	Message  string         `json:"message"`
	Options  []PromptOption `json:"options,omitempty"` // For multi-select
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
	p.rc = make(chan PromptResponse)
	for prompt := range p.c {
		timeout := time.After(time.Until(prompt.Deadline))
		select {
		case <-timeout:
			prompt.resp <- PromptResponse{ID: prompt.ID, Err: fmt.Errorf("deadline reached")}
			close(prompt.cancel)
		case resp := <-p.rc:
			if resp.ID != prompt.ID {
				continue
			}
			select {
			case prompt.resp <- resp:
			default:
			}
			close(prompt.cancel)
		}
	}
}

func (p *PromptHub) Respond(id, value string, err error) {
	select {
	case p.rc <- PromptResponse{ID: id, Value: value, Err: err}:
	default:
	}
}

func (p *PromptHub) Prompt(kind PromptKind, message string, options ...PromptOption) <-chan PromptResponse {
	prompt := &Prompt{
		resp:     make(chan PromptResponse),
		cancel:   make(chan struct{}), // Closed on cancel (e.g. prompt response received)
		ID:       fmt.Sprint(time.Now().UnixNano()),
		Kind:     kind,
		Message:  message,
		Options:  options,
		Deadline: time.Now().Add(time.Minute),
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
		case <-prompt.cancel:
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
		ans := strings.FieldsFunc(readLine(), func(r rune) bool { return r == ' ' || r == ',' })
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
	default:
		panic(prompt.Kind + " prompt not implemented")
	}
}
