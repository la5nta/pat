// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/la5nta/pat/app"
	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/pat/internal/editor"
	"github.com/la5nta/wl2k-go/fbb"
	"github.com/la5nta/wl2k-go/mailbox"

	"github.com/spf13/pflag"
)

func postMessage(a *app.App, msg *fbb.Message) {
	if err := msg.Validate(); err != nil {
		fmt.Printf("WARNING - Message does not validate: %s\n", err)
	}
	if err := a.Mailbox().AddOut(msg); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Message posted")
}

func openMessage(a *app.App, path string) (*fbb.Message, error) {
	// Search if only MID is specified.
	if filepath.Dir(path) == "." && filepath.Ext(path) == "" {
		debug.Printf("openMessage(%q): Searching...", path)
		path += mailbox.Ext
		fs.WalkDir(os.DirFS(a.Mailbox().MBoxPath), ".", func(p string, d fs.DirEntry, err error) error {
			if d.Name() != path {
				return nil
			}
			debug.Printf("openMessage(%q): Found %q", d.Name(), p)
			path = filepath.Join(a.Mailbox().MBoxPath, p)
			return io.EOF
		})
	}
	return mailbox.OpenMessage(path)
}

func composeMessageHeader(app *app.App, opts *ComposeOpts) *fbb.Message {
	msg := fbb.NewMessage(fbb.Private, app.Options().MyCall)

	fmt.Printf(`From [%s]: `, app.Options().MyCall)
	from := readLine()
	if from == "" {
		from = app.Options().MyCall
	}
	msg.SetFrom(from)

	fmt.Print(`To`)
	if opts.OriginalMsg != nil {
		fmt.Printf(" [%s]", opts.OriginalMsg.From())
	}
	fmt.Printf(": ")
	to := readLine()
	if to == "" && opts.OriginalMsg != nil {
		msg.AddTo(opts.OriginalMsg.From().String())
	} else {
		for _, addr := range strings.FieldsFunc(to, SplitFunc) {
			msg.AddTo(addr)
		}
	}

	ccCand := make([]fbb.Address, 0)
	if opts.Action == ComposeActionReplyAll {
		for _, addr := range append(opts.OriginalMsg.To(), opts.OriginalMsg.Cc()...) {
			if !addr.EqualString(app.Options().MyCall) {
				ccCand = append(ccCand, addr)
			}
		}
	}

	fmt.Printf("Cc")
	if len(ccCand) > 0 {
		fmt.Printf(" %s", ccCand)
	}
	fmt.Print(`: `)
	cc := readLine()
	if cc == "" {
		for _, addr := range ccCand {
			msg.AddCc(addr.String())
		}
	} else {
		for _, addr := range strings.FieldsFunc(cc, SplitFunc) {
			msg.AddCc(addr)
		}
	}

	switch len(msg.Receivers()) {
	case 1:
		fmt.Print("P2P only [y/N]: ")
		ans := readLine()
		if strings.EqualFold("y", ans) {
			msg.Header.Set("X-P2POnly", "true")
		}
	case 0:
		fmt.Println("Message must have at least one recipient")
		os.Exit(1)
	}

	fmt.Print(`Subject: `)
	switch opts.Action {
	case ComposeActionForward:
		subject := strings.TrimSpace(strings.TrimPrefix(opts.OriginalMsg.Subject(), "Fwd:"))
		subject = fmt.Sprintf("Fwd:%s", subject)
		fmt.Println(subject)
		msg.SetSubject(subject)
	case ComposeActionReplyAll, ComposeActionReply:
		subject := strings.TrimSpace(strings.TrimPrefix(opts.OriginalMsg.Subject(), "Re:"))
		subject = fmt.Sprintf("Re:%s", subject)
		fmt.Println(subject)
		msg.SetSubject(subject)
	default:
		msg.SetSubject(readLine())
	}
	// A message without subject is not valid, so let's use a sane default
	if msg.Subject() == "" {
		msg.SetSubject("<No subject>")
	}

	return msg
}

func ComposeMessage(ctx context.Context, app *app.App, args []string) {
	exitOnContextCancellation(ctx)

	set := pflag.NewFlagSet("compose", pflag.ExitOnError)
	// From default is --mycall but it can be overriden with -r
	from := set.StringP("from", "r", app.Options().MyCall, "")
	subject := set.StringP("subject", "s", "", "")
	attachments := set.StringArrayP("attachment", "a", nil, "")
	ccs := set.StringArrayP("cc", "c", nil, "")
	p2pOnly := set.BoolP("p2p-only", "", false, "")
	template := set.StringP("template", "", "", "")
	inReplyTo := set.StringP("in-reply-to", "", "", "")
	replyAll := set.BoolP("reply-all", "", false, "")
	set.Parse(args)

	// Remaining args are recipients
	recipients := []string{}
	for _, r := range set.Args() {
		// Filter out empty args (this actually happens)
		if strings.TrimSpace(r) == "" {
			continue
		}
		recipients = append(recipients, r)
	}

	// Load in-reply-to message
	var inReplyToMsg *fbb.Message
	if path := *inReplyTo; path != "" {
		var err error
		inReplyToMsg, err = openMessage(app, path)
		if err != nil {
			log.Fatal(err)
		}
	}

	opts := &ComposeOpts{Action: ComposeActionNew}
	if inReplyToMsg != nil {
		opts.Action = ComposeActionReply
		if *replyAll {
			opts.Action = ComposeActionReplyAll
		}
		opts.OriginalMsg = inReplyToMsg
	}

	// Check if condition are met for non-interactive compose.
	if (len(*subject)+len(*attachments)+len(*ccs)+len(recipients)) > 0 && *template != "" {
		noninteractiveComposeMessage(app, *from, *subject, *attachments, *ccs, recipients, *p2pOnly)
		return
	}

	// Use template?
	if *template != "" {
		interactiveComposeWithTemplate(app, *template, opts)
		return
	}

	// Interactive compose
	InteractiveComposeMessage(app, opts)
}

func noninteractiveComposeMessage(app *app.App, from string, subject string, attachments []string, ccs []string, recipients []string, p2pOnly bool) {
	// We have to verify the args here. Follow the same pattern as main()
	// We'll allow a missing recipient if CC is present (or vice versa)
	if len(recipients)+len(ccs) <= 0 {
		fmt.Fprint(os.Stderr, "ERROR: Missing recipients in non-interactive mode!\n")
		os.Exit(1)
	}

	// Subject is optional. Print a mailx style warning
	if subject == "" {
		fmt.Fprint(os.Stderr, "Warning: missing subject; hope that's OK\n")
	}

	msg := fbb.NewMessage(fbb.Private, app.Options().MyCall)
	msg.SetFrom(from)
	for _, to := range recipients {
		msg.AddTo(to)
	}
	for _, cc := range ccs {
		msg.AddCc(cc)
	}

	msg.SetSubject(subject)

	// Handle Attachments. Since we're not interactive, treat errors as fatal so the user can fix
	for _, path := range attachments {
		if err := addAttachmentFromPath(msg, path); err != nil {
			fmt.Fprint(os.Stderr, err.Error()+"\nAborting! (Message not posted)\n")
			os.Exit(1)
		}
	}

	// Read the message body from stdin
	body, _ := io.ReadAll(os.Stdin)
	if len(body) == 0 {
		// Yeah, I've spent way too much time using mail(1)
		fmt.Fprint(os.Stderr, "Null message body; hope that's ok\n")
	}

	msg.SetBody(string(body))
	if p2pOnly {
		msg.Header.Set("X-P2POnly", "true")
	}

	postMessage(app, msg)
}

func composeBody(template string) (string, error) {
	body, err := editor.EditText(template)
	if err != nil {
		return body, err
	}
	// An empty message body is illegal. Let's set a sane default.
	if len(strings.TrimSpace(body)) == 0 {
		body = "<No message body>\n"
	}
	return body, nil
}

func InteractiveComposeMessage(app *app.App, opts *ComposeOpts) {
	msg := composeMessageHeader(app, opts)

	// Body
	var template bytes.Buffer
	switch opts.Action {
	case ComposeActionForward, ComposeActionReply, ComposeActionReplyAll:
		writeMessageCitation(&template, opts.OriginalMsg)
	}
	fmt.Printf(`Press ENTER to start composing the message body. `)
	readLine()
	body, err := composeBody(template.String())
	if err != nil {
		log.Fatal(err)
	}
	msg.SetBody(body)

	// Attachments
	if opts.Action == ComposeActionForward {
		for _, f := range opts.OriginalMsg.Files() {
			msg.AddFile(f)
		}
	}
	fmt.Print("\n")
	for {
		fmt.Print(`Attachment [empty when done]: `)
		path := readLine()
		if path == "" {
			break
		}
		if err := addAttachmentFromPath(msg, path); err != nil {
			log.Println(err)
			continue
		}
	}
	fmt.Println(msg)
	postMessage(app, msg)
}

func writeMessageCitation(w io.Writer, inReplyToMsg *fbb.Message) {
	fmt.Fprintf(w, "--- %s %s wrote: ---\n", inReplyToMsg.Date(), inReplyToMsg.From().Addr)
	body, _ := inReplyToMsg.Body()
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		fmt.Fprintf(w, ">%s\n", scanner.Text())
	}
}

func addAttachmentFromPath(msg *fbb.Message, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return app.AddAttachment(msg, filepath.Base(path), "", f)
}

func interactiveComposeWithTemplate(a *app.App, template string, opts *ComposeOpts) {
	msg := composeMessageHeader(a, opts)

	formMsg, err := a.FormsManager().ComposeTemplate(template, msg.Subject(), opts.OriginalMsg, readLine)
	if err != nil {
		log.Printf("failed to compose message for template: %v", err)
		return
	}

	msg.SetSubject(formMsg.Subject)
	for _, f := range formMsg.Attachments {
		msg.AddFile(f)
	}

	fmt.Println("================================================================")
	fmt.Print("To: ")
	fmt.Println(msg.To())
	fmt.Print("Cc: ")
	fmt.Println(msg.Cc())
	fmt.Print("From: ")
	fmt.Println(msg.From())
	fmt.Println("Subject: " + msg.Subject())
	fmt.Println("================================================================")
	fmt.Println(formMsg.Body)
	fmt.Println("================================================================")
L:
	for {
		fmt.Print("Post message to outbox? [Y,q,e,?]: ")
		switch readLine() {
		case "Y", "y", "":
			break L
		case "e":
			var err error
			if formMsg.Body, err = composeBody(formMsg.Body); err != nil {
				log.Fatal(err)
			}
		case "q":
			return
		case "?":
			fallthrough
		default:
			fmt.Println("y = post message to outbox")
			fmt.Println("e = edit message body")
			fmt.Println("q = quit, discarding the message")
		}
	}
	msg.SetBody(formMsg.Body)
	postMessage(a, msg)
}

type ComposeAction int

const (
	ComposeActionNew ComposeAction = iota
	ComposeActionReply
	ComposeActionReplyAll
	ComposeActionForward
)

type ComposeOpts struct {
	Action      ComposeAction
	OriginalMsg *fbb.Message
}
