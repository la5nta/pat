package cli

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/la5nta/pat/app"

	"github.com/howeyc/gopass"
)

type TerminalPrompter struct{}

func (t TerminalPrompter) Prompt(prompt app.Prompt) {
	q := make(chan struct{}, 1)
	defer close(q)
	go func() {
		select {
		case <-prompt.Done():
			fmt.Printf(" Prompt Aborted - Press ENTER to continue...")
		case <-q:
			return
		}
	}()

	switch prompt.Kind {
	case app.PromptKindMultiSelect:
		fmt.Println(prompt.Message + ":")
		answers := map[string]app.PromptOption{}
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
		prompt.Respond(strings.Join(selected, ","), nil)
	case app.PromptKindPassword:
		passwd, err := gopass.GetPasswdPrompt(prompt.Message+": ", true, os.Stdin, os.Stdout)
		prompt.Respond(string(passwd), err)
	case app.PromptKindBusyChannel:
		fmt.Println(prompt.Message + ":")
		for prompt.Err() == nil {
			fmt.Printf("Answer [c(ontinue), a(bort)]: ")
			switch ans := readLine(); strings.TrimSpace(ans) {
			case "c", "continue":
				prompt.Respond("continue", nil)
				return
			case "a", "abort":
				prompt.Respond("abort", nil)
				return
			}
		}
	case app.PromptKindPreAccountActivation:
		fmt.Println()
		fmt.Println("WARNING: We were unable to confirm that your Winlink account is active.")
		fmt.Println("If you continue, an over-the-air activation will be initiated and you will receive a message with a new password.")
		fmt.Println("This password will be the only key to your account. If you lose it, it cannot be recovered.")
		fmt.Printf("It is strongly recommended to use '%s init' or the web gui to create your account before proceeding.\n", os.Args[0])
		fmt.Println()
		for prompt.Err() == nil {
			fmt.Printf("Answer [c(ontinue), a(bort)]: ")
			switch ans := readLine(); strings.TrimSpace(ans) {
			case "c", "continue":
				prompt.Respond("confirmed", nil)
				return
			case "a", "abort":
				prompt.Respond("abort", nil)
				return
			}
		}
	case app.PromptKindAccountActivation:
		fmt.Println()
		fmt.Println("WARNING:")
		fmt.Println("You are about to receive a computer-generated password for your new Winlink account.")
		fmt.Println("Once you download this message, the password inside is the only key to your account.")
		fmt.Println("If you lose it, it cannot be recovered.")
		fmt.Println()
		fmt.Println("Are you ready to receive this message and save the password securely right now?")
		for prompt.Err() == nil {
			fmt.Printf("Answer (yes/no): ")
			switch ans := readLine(); strings.TrimSpace(ans) {
			case "y", "yes":
				prompt.Respond("accept", nil)
				return
			case "n", "no":
				prompt.Respond("defer", nil)
				return
			}
		}
	default:
		log.Printf("Prompt kind %q not implemented", prompt.Kind)
	}
}
