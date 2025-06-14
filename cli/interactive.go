package cli

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/la5nta/pat/api"
	"github.com/la5nta/pat/app"
	"github.com/la5nta/wl2k-go/rigcontrol/hamlib"
	"github.com/peterh/liner"

	"github.com/spf13/pflag"
)

func InteractiveHandle(ctx context.Context, a *app.App, args []string) {
	var http string
	set := pflag.NewFlagSet("interactive", pflag.ExitOnError)
	set.StringVar(&http, "http", "", "HTTP listen address")
	set.Lookup("http").NoOptDefVal = a.Config().HTTPAddr
	set.Parse(args)

	a.PromptHub().AddPrompter(TerminalPrompter{})

	if http == "" {
		Interactive(ctx, a)
		return
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		if err := api.ListenAndServe(ctx, a, http); err != nil {
			log.Println(err)
		}
	}()
	time.Sleep(time.Second)
	Interactive(ctx, a)
}

func Interactive(ctx context.Context, a *app.App) {
	scheduleLoop(ctx, a)

	line := liner.NewLiner()
	defer line.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			str, _ := line.Prompt(getPrompt(a))
			if str == "" {
				continue
			}
			line.AppendHistory(str)

			if str[0] == '#' {
				continue
			}

			if quit := execCmd(a, str); quit {
				break
			}
		}
	}()
	select {
	case <-ctx.Done():
	case <-done:
	}
}

func execCmd(a *app.App, line string) (quit bool) {
	cmd, param := parseCommand(line)
	switch cmd {
	case "connect":
		if param == "" {
			printInteractiveUsage()
			return
		}

		a.Connect(param)
	case "listen":
		a.Listen(param)
	case "unlisten":
		a.Unlisten(param)
	case "heard":
		PrintHeard(a)
	case "freq":
		freq(a, param)
	case "qtc":
		PrintQTC(a)
	case "debug":
		os.Setenv("ardop_debug", "1")
		fmt.Println("Number of goroutines:", runtime.NumGoroutine())
	case "q", "quit":
		return true
	case "":
		return
	default:
		printInteractiveUsage()
	}
	return
}

func printInteractiveUsage() {
	fmt.Println("Uri examples: 'LA3F@5350', 'LA1B-10 v LA5NTA-1', 'LA5NTA:secret@192.168.1.1:54321'")

	transports := []string{
		app.MethodArdop,
		app.MethodAX25, app.MethodAX25AGWPE, app.MethodAX25Linux, app.MethodAX25SerialTNC,
		app.MethodPactor,
		app.MethodTelnet,
		app.MethodVaraHF,
		app.MethodVaraFM,
	}
	fmt.Println("Transports:", strings.Join(transports, ", "))

	cmds := []string{
		"connect  <connect-url or alias>  Connect to a remote station.",
		"listen   <transport>             Listen for incoming connections.",
		"unlisten <transport>             Unregister listener for incoming connections.",
		"freq     <transport>[:<freq>]    Read/set rig frequency.",
		"heard                            Display all stations heard over the air.",
		"qtc                              Print pending outbound messages.",
	}
	fmt.Println("Commands: ")
	for _, cmd := range cmds {
		fmt.Printf(" %s\n", cmd)
	}
}

func getPrompt(a *app.App) string {
	var buf bytes.Buffer

	if listeners := a.ActiveListeners(); len(listeners) > 0 {
		fmt.Fprintf(&buf, "L%v", listeners)
	}

	fmt.Fprint(&buf, "> ")
	return buf.String()
}

func PrintHeard(a *app.App) {
	pf := func(call string, t time.Time) {
		fmt.Printf("  %-10s (%s)\n", call, t.Format(time.RFC1123))
	}

	for method, heard := range a.Heard() {
		fmt.Printf("%s:\n", method)
		for _, v := range heard {
			pf(v.Callsign, v.Time)
		}
	}
}

func PrintQTC(a *app.App) {
	msgs, err := a.Mailbox().Outbox()
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Printf("QTC: %d.\n", len(msgs))
	for _, msg := range msgs {
		fmt.Printf(`%-12.12s (%s): %s`, msg.MID(), msg.Subject(), fmt.Sprint(msg.To()))
		if msg.Header.Get("X-P2POnly") == "true" {
			fmt.Printf(" (P2P only)")
		}
		fmt.Println("")
	}
}

func freq(a *app.App, param string) {
	parts := strings.SplitN(param, ":", 2)
	if parts[0] == "" {
		fmt.Println("Missing transport parameter.")
		fmt.Println("Syntax: freq <transport>[:<frequency>]")
		return
	}

	rig, rigName, ok, err := a.VFOForTransport(parts[0])
	if err != nil {
		log.Println(err)
		return
	} else if !ok {
		log.Printf("Rig '%s' not loaded.", rigName)
		return
	}

	if len(parts) < 2 {
		freq, err := rig.GetFreq()
		if err != nil {
			log.Printf("Unable to get frequency: %s", err)
		}
		fmt.Printf("%.3f\n", float64(freq)/1e3)
		return
	}

	if _, _, err := setFreq(rig, parts[1]); err != nil {
		log.Printf("Unable to set frequency: %s", err)
	}
}

func setFreq(rig hamlib.VFO, freq string) (newFreq, oldFreq int, err error) {
	oldFreq, err = rig.GetFreq()
	if err != nil {
		return 0, 0, fmt.Errorf("unable to get rig frequency: %w", err)
	}

	f, err := strconv.ParseFloat(freq, 64)
	if err != nil {
		return 0, 0, err
	}

	newFreq = int(f * 1e3)
	err = rig.SetFreq(newFreq)
	return
}

func parseCommand(str string) (mode, param string) {
	parts := strings.SplitN(str, " ", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}
