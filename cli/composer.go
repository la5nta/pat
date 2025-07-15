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
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/la5nta/pat/app"
	"github.com/la5nta/pat/internal/editor"
	"github.com/la5nta/wl2k-go/fbb"

	"github.com/spf13/pflag"
)

type composerFlags struct {
	// Headers
	from    string
	to      []string
	cc      []string
	subject string
	p2pOnly bool

	body string

	attachmentPaths []string
	attachments     []*fbb.File // forwarded attachments and/or attachments generated from template

	template  string // path to template/form
	forward   string // path/mid
	inReplyTo string // path/mid
	replyAll  bool
}

func ComposeMessage(ctx context.Context, app *app.App, args []string) {
	exitOnContextCancellation(ctx)

	var flags composerFlags
	set := pflag.NewFlagSet("compose", pflag.ExitOnError)
	set.StringVarP(&flags.from, "from", "r", app.Options().MyCall, "")
	set.StringVarP(&flags.subject, "subject", "s", "", "")
	set.StringArrayVarP(&flags.attachmentPaths, "attachment", "a", nil, "")
	set.StringArrayVarP(&flags.cc, "cc", "c", nil, "")
	set.BoolVarP(&flags.p2pOnly, "p2p-only", "", false, "")
	set.StringVarP(&flags.template, "template", "", "", "")
	set.StringVarP(&flags.inReplyTo, "in-reply-to", "", "", "")
	set.StringVarP(&flags.forward, "forward", "", "", "")
	set.BoolVarP(&flags.replyAll, "reply-all", "", false, "")
	set.Parse(args)
	// Remaining args are recipients
	for _, r := range set.Args() {
		if strings.TrimSpace(r) == "" { // Filter out empty args (this actually happens)
			continue
		}
		flags.to = append(flags.to, r)
	}

	composeMessage(app, flags, isTerminal(os.Stdin))
}

func composeMessage(app *app.App, flags composerFlags, interactive bool) {
	switch {
	case flags.inReplyTo != "" && flags.forward != "":
		log.Fatal("reply and forward are mutually exclusive operations")
	case flags.inReplyTo != "":
		if err := prepareReply(app, &flags); err != nil {
			log.Fatal(err)
		}
	case flags.forward != "":
		if err := prepareForward(app, &flags); err != nil {
			log.Fatal(err)
		}
	}

	if interactive {
		promptHeader(&flags)
	}

	if err := buildBody(app, &flags, interactive); err != nil {
		log.Fatal(err)
	}

	if interactive {
		promptAttachments(&flags.attachmentPaths)
		if !previewAndPromptConfirmation(&flags) {
			return
		}
	}

	msg := buildMessage(app.Options().MyCall, flags)
	postMessage(app, msg)
}

func prepareReply(app *app.App, flags *composerFlags) error {
	originalMsg, err := openMessage(app, flags.inReplyTo)
	if err != nil {
		return err
	}
	if flags.subject == "" {
		flags.subject = "Re: " + strings.TrimSpace(strings.TrimPrefix(originalMsg.Subject(), "Re:"))
	}
	flags.to = append(flags.to, originalMsg.From().String())
	if flags.replyAll {
		for _, addr := range append(originalMsg.To(), originalMsg.Cc()...) {
			if !addr.EqualString(app.Options().MyCall) {
				flags.cc = append(flags.cc, addr.String())
			}
		}
	}
	var buf bytes.Buffer
	writeMessageCitation(&buf, originalMsg)
	flags.body = buf.String()
	return nil
}

func prepareForward(app *app.App, flags *composerFlags) error {
	originalMsg, err := openMessage(app, flags.forward)
	if err != nil {
		return err
	}
	if flags.subject == "" {
		flags.subject = "Fwd: " + strings.TrimSpace(strings.TrimPrefix(originalMsg.Subject(), "Fwd:"))
	}
	flags.attachments = append(flags.attachments, originalMsg.Files()...)
	var buf bytes.Buffer
	writeMessageCitation(&buf, originalMsg)
	flags.body = buf.String()
	return nil
}

func buildBody(app *app.App, flags *composerFlags, interactive bool) error {
	switch {
	case flags.template != "":
		inReplyTo, err := openMessage(app, flags.inReplyTo)
		if err != nil && flags.inReplyTo != "" {
			return err
		}
		formMsg, err := app.FormsManager().ComposeTemplate(flags.template, flags.subject, inReplyTo, readLine)
		if err != nil {
			return fmt.Errorf("failed to compose message for template: %w", err)
		}
		flags.subject = formMsg.Subject
		flags.body = formMsg.Body
		flags.attachments = append(flags.attachments, formMsg.Attachments...)
	case interactive:
		promptBody(&flags.body)
	default:
		// Read body from stdin
		body, _ := io.ReadAll(os.Stdin)
		if len(body) == 0 {
			fmt.Fprint(os.Stderr, "Null message body; hope that's ok\n")
		}
		flags.body = string(body)
	}
	return nil
}

func postMessage(a *app.App, msg *fbb.Message) {
	if err := msg.Validate(); err != nil {
		fmt.Printf("WARNING - Message does not validate: %s\n", err)
	}
	if err := a.Mailbox().AddOut(msg); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Message posted")
}

func promptHeader(flags *composerFlags) {
	flags.from = prompt("From", flags.from)
	flags.to = strings.FieldsFunc(prompt("To", strings.Join(flags.to, ",")), SplitFunc)
	flags.cc = strings.FieldsFunc(prompt("Cc", strings.Join(flags.cc, ",")), SplitFunc)
	switch len(flags.to) + len(flags.cc) {
	case 1:
		if flags.p2pOnly {
			flags.p2pOnly = strings.EqualFold(prompt("P2P only", "Y", "n"), "y")
		} else {
			flags.p2pOnly = strings.EqualFold(prompt("P2P only", "N", "y"), "y")
		}
	case 0:
		fmt.Println("Message must have at least one recipient")
		os.Exit(1)
	}
	flags.subject = prompt("Subject", flags.subject)
	// A message without subject is not valid, so let's use a sane default
	if flags.subject == "" {
		flags.subject = "<No subject>"
	}
}

func promptBody(body *string) {
	fmt.Printf(`Press ENTER to start composing the message body. `)
	readLine()
	var err error
	*body, err = composeBody(*body)
	if err != nil {
		log.Fatal(err)
	}
}

func promptAttachments(attachmentPaths *[]string) {
	for _, path := range *attachmentPaths {
		fmt.Println("Attachment [empty when done]:", path)
	}
	for {
		path := prompt("Attachment [empty when done]", "")
		if path == "" {
			break
		}
		if _, err := os.Stat(path); err != nil {
			log.Println(err)
			continue
		}
		*attachmentPaths = append(*attachmentPaths, path)
	}
}

func buildMessage(mycall string, flags composerFlags) *fbb.Message {
	// We have to verify the args here. Follow the same pattern as main()
	// We'll allow a missing recipient if CC is present (or vice versa)
	if len(flags.to)+len(flags.cc) <= 0 {
		fmt.Fprint(os.Stderr, "ERROR: Missing recipients in non-interactive mode!\n")
		os.Exit(1)
	}

	msg := fbb.NewMessage(fbb.Private, mycall)
	msg.SetFrom(flags.from)
	for _, to := range flags.to {
		msg.AddTo(to)
	}
	for _, cc := range flags.cc {
		msg.AddCc(cc)
	}

	// Subject is optional. Print a mailx style warning
	if flags.subject == "" {
		fmt.Fprint(os.Stderr, "Warning: missing subject; hope that's OK\n")
	}
	msg.SetSubject(flags.subject)

	// Handle Attachments. Since we're not interactive, treat errors as fatal so the user can fix
	for _, path := range flags.attachmentPaths {
		if err := addAttachmentFromPath(msg, path); err != nil {
			fmt.Fprint(os.Stderr, err.Error()+"\nAborting! (Message not posted)\n")
			os.Exit(1)
		}
	}
	for _, f := range flags.attachments {
		msg.AddFile(f)
	}

	msg.SetBody(flags.body)
	if flags.p2pOnly {
		msg.Header.Set("X-P2POnly", "true")
	}

	return msg
}

func previewAndPromptConfirmation(flags *composerFlags) (ok bool) {
	preview := func() {
		var attachments []string
		for _, a := range flags.attachments {
			attachments = append(attachments, a.Name())
		}
		attachments = append(attachments, flags.attachmentPaths...)
		fmt.Println()
		fmt.Println("================================================================")
		fmt.Println("To:", strings.Join(flags.to, ", "))
		fmt.Println("Cc:", strings.Join(flags.cc, ", "))
		fmt.Println("From:", flags.from)
		fmt.Println("Subject:", flags.subject)
		fmt.Println("Attachments:", strings.Join(attachments, ", "))
		fmt.Println("================================================================")
		fmt.Println(flags.body)
		fmt.Println("================================================================")
	}

	preview()
	for {
		fmt.Print("Post message to outbox? [Y,q,e,?]: ")
		switch readLine() {
		case "Y", "y", "":
			return true
		case "e":
			flags.body, _ = composeBody(flags.body)
			preview()
		case "q":
			return false
		case "?":
			fallthrough
		default:
			fmt.Println("y = post message to outbox")
			fmt.Println("e = edit message body")
			fmt.Println("q = quit, discarding the message")
		}
	}
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
