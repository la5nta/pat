// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// A portable Winlink client for amateur radio email.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/la5nta/pat/internal/buildinfo"

	"github.com/la5nta/wl2k-go/fbb"
	"github.com/spf13/pflag"
)

func composeMessageHeader(replyMsg *fbb.Message) *fbb.Message {
	msg := fbb.NewMessage(fbb.Private, fOptions.MyCall)

	fmt.Printf(`From [%s]: `, fOptions.MyCall)
	from := readLine()
	if from == "" {
		from = fOptions.MyCall
	}
	msg.SetFrom(from)

	fmt.Print(`To`)
	if replyMsg != nil {
		fmt.Printf(" [%s]", replyMsg.From())
	}
	fmt.Printf(": ")
	to := readLine()
	if to == "" && replyMsg != nil {
		msg.AddTo(replyMsg.From().String())
	} else {
		for _, addr := range strings.FieldsFunc(to, SplitFunc) {
			msg.AddTo(addr)
		}
	}

	ccCand := make([]fbb.Address, 0)
	if replyMsg != nil {
		for _, addr := range append(replyMsg.To(), replyMsg.Cc()...) {
			if !addr.EqualString(fOptions.MyCall) {
				ccCand = append(ccCand, addr)
			}
		}
	}

	fmt.Printf("Cc")
	if replyMsg != nil {
		fmt.Printf(" %s", ccCand)
	}
	fmt.Print(`: `)
	cc := readLine()
	if cc == "" && replyMsg != nil {
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
	if replyMsg != nil {
		subject := strings.TrimSpace(strings.TrimPrefix(replyMsg.Subject(), "Re:"))
		subject = fmt.Sprintf("Re:%s", subject)
		fmt.Println(subject)
		msg.SetSubject(subject)
	} else {
		msg.SetSubject(readLine())
	}
	// A message without subject is not valid, so let's use a sane default
	if msg.Subject() == "" {
		msg.SetSubject("<No subject>")
	}

	return msg
}

func composeMessage(args []string) {
	set := pflag.NewFlagSet("compose", pflag.ExitOnError)
	// From default is --mycall but it can be overriden with -r
	from := set.StringP("from", "r", fOptions.MyCall, "")
	subject := set.StringP("subject", "s", "", "")
	attachments := set.StringArrayP("attachment", "a", nil, "")
	ccs := set.StringArrayP("cc", "c", nil, "")
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

	// Check if any args are set. If so, go non-interactive
	// Otherwise, interactive
	if (len(*subject) + len(*attachments) + len(*ccs) + len(recipients)) > 0 {
		noninteractiveComposeMessage(*from, *subject, *attachments, *ccs, recipients)
	} else {
		interactiveComposeMessage(nil)
	}
}

func noninteractiveComposeMessage(from string, subject string, attachments []string,
	ccs []string, recipients []string) {
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

	msg := fbb.NewMessage(fbb.Private, fOptions.MyCall)
	msg.SetFrom(from)
	for _, to := range recipients {
		msg.AddTo(to)
	}
	for _, cc := range ccs {
		msg.AddCc(cc)
	}

	msg.SetSubject(subject)

	// Handle Attachments. Since we're not interactive, treat errors as fatal so the user can fix
	for _, filename := range attachments {
		file, err := readAttachment(filename)
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error()+"\nAborting! (Message not posted)\n")
			os.Exit(1)
		}
		msg.AddFile(file)
	}

	// Read the message body from stdin
	body, _ := ioutil.ReadAll(os.Stdin)
	if len(body) == 0 {
		// Yeah, I've spent way too much time using mail(1)
		fmt.Fprint(os.Stderr, "Null message body; hope that's ok\n")
	}

	msg.SetBody(string(body))

	postMessage(msg)
}

// This is currently an alias for interactiveComposeMessage but keeping as a separate
// call path for the future
func composeReplyMessage(replyMsg *fbb.Message) {
	interactiveComposeMessage(replyMsg)
}

func interactiveComposeMessage(replyMsg *fbb.Message) {
	msg := composeMessageHeader(replyMsg)

	// Read body
	fmt.Printf(`Press ENTER to start composing the message body. `)
	readLine()

	f, err := ioutil.TempFile("", strings.ToLower(fmt.Sprintf("%s_new_%d.txt", buildinfo.AppName, time.Now().Unix())))
	if err != nil {
		log.Fatalf("Unable to prepare temporary file for body: %s", err)
	}

	if replyMsg != nil {
		fmt.Fprintf(f, "--- %s %s wrote: ---\n", replyMsg.Date(), replyMsg.From().Addr)
		body, _ := replyMsg.Body()
		orig := ">" + strings.ReplaceAll(
			strings.TrimSpace(body),
			"\n",
			"\n>",
		) + "\n"
		f.Write([]byte(orig))
		f.Sync()
	}

	// Windows fix: Avoid 'cannot access the file because it is being used by another process' error.
	// Close the file before opening the editor.
	f.Close()

	cmd := exec.Command(EditorName(), f.Name())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("Unable to start body editor: %s", err)
	}

	f, err = os.OpenFile(f.Name(), os.O_RDWR, 0o666)
	if err != nil {
		log.Fatalf("Unable to read temporary file from editor: %s", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, f)
	msg.SetBody(buf.String())
	f.Close()
	os.Remove(f.Name())

	// An empty message body is illegal. Let's set a sane default.
	if msg.BodySize() == 0 {
		msg.SetBody("<No message body>\n")
	}

	// END Read body

	fmt.Print("\n")

	for {
		fmt.Print(`Attachment [empty when done]: `)
		path := readLine()
		if path == "" {
			break
		}

		file, err := readAttachment(path)
		if err != nil {
			log.Println(err)
			continue
		}

		msg.AddFile(file)
	}
	fmt.Println(msg)
	postMessage(msg)
}

func readAttachment(path string) (*fbb.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	name := filepath.Base(path)

	var resizeImage bool
	if isImageMediaType(name, "") {
		fmt.Print("This seems to be an image. Auto resize? [Y/n]: ")
		ans := readLine()
		resizeImage = ans == "" || strings.EqualFold("y", ans)
	}

	var data []byte

	data, err = ioutil.ReadAll(f)
	if resizeImage {
		data, err = convertImage(data)
		ext := filepath.Ext(name)
		name = name[:len(name)-len(ext)] + ".jpg"
	}

	return fbb.NewFile(name, data), err
}

var stdin *bufio.Reader

func readLine() string {
	if stdin == nil {
		stdin = bufio.NewReader(os.Stdin)
	}

	str, _ := stdin.ReadString('\n')
	return strings.TrimSpace(str)
}

func composeFormReport(args []string) {
	var tmplPathArg string

	set := pflag.NewFlagSet("form", pflag.ExitOnError)
	set.StringVar(&tmplPathArg, "template", "ICS USA Forms/ICS213", "")
	set.Parse(args)

	msg := composeMessageHeader(nil)

	formMsg, err := formsMgr.ComposeForm(tmplPathArg, msg.Subject())
	if err != nil {
		log.Printf("failed to compose message for template %s", tmplPathArg)
		return
	}

	msg.SetSubject(formMsg.Subject)

	fmt.Println("================================================================")
	fmt.Print("To: ")
	fmt.Println(msg.To())
	fmt.Print("Cc: ")
	fmt.Println(msg.Cc())
	fmt.Print("From: ")
	fmt.Println(msg.From())
	fmt.Println("Subject: " + msg.Subject())
	fmt.Println(formMsg.Body)
	fmt.Println("================================================================")
	fmt.Println("Press ENTER to post this message in the outbox, Ctrl-C to abort.")
	fmt.Println("================================================================")
	readLine()

	msg.SetBody(formMsg.Body)

	attachmentFile := fbb.NewFile(formMsg.AttachmentName, []byte(formMsg.AttachmentXML))
	msg.AddFile(attachmentFile)

	postMessage(msg)
}
