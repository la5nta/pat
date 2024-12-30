package main

import (
	"fmt"
	"os"
	"time"

	"github.com/howeyc/gopass"
)

type Prompt struct {
	resp     chan PromptResponse
	cancel   chan struct{}
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	Deadline time.Time `json:"deadline"`
	Message  string    `json:"message"`
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

func (p *PromptHub) Prompt(kind, message string) <-chan PromptResponse {
	prompt := &Prompt{
		resp:     make(chan PromptResponse),
		cancel:   make(chan struct{}), // Closed on cancel (e.g. prompt response received)
		ID:       fmt.Sprint(time.Now().UnixNano()),
		Kind:     kind,
		Message:  message,
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
	switch prompt.Kind {
	case "password":
		q := make(chan struct{}, 1)
		go func() {
			select {
			case <-prompt.cancel:
				fmt.Printf(" Prompt Aborted - Press ENTER to continue...")
			case <-q:
				return
			}
		}()
		passwd, err := gopass.GetPasswdPrompt(prompt.Message+": ", true, os.Stdin, os.Stdout)
		q <- struct{}{}
		p.Respond(prompt.ID, string(passwd), err)
	default:
		panic(prompt.Kind + " prompt not implemented")
	}
}
