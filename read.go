// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/bndr/gotabulate"

	"github.com/la5nta/wl2k-go"
	"github.com/la5nta/wl2k-go/mailbox"
)

var mailboxes = []string{"in", "out", "sent", "archive"}

func readMail() {
	w := os.Stdout
	rd := bufio.NewReader(os.Stdin)

	// Query user for mailbox to list
	printMailboxes(w)
	fmt.Fprintf(w, "\nChoose mailbox [0]: ")
	mailboxIdx := readInt(rd)
	if mailboxIdx+1 > len(mailboxes) {
		fmt.Fprintln(w, "Invalid mailbox number")
		return
	}

	for {
		// Fetch messages
		msgs, err := mailbox.LoadMessageDir(path.Join(mailbox.UserPath(fOptions.MailboxPath, fOptions.MyCall), mailboxes[mailboxIdx]))
		if err != nil {
			log.Fatal(err)
		} else if len(msgs) == 0 {
			fmt.Fprintf(w, "(empty)\n")
			break
		}

		// Print messages (sorted by date)
		sort.Sort(wl2k.ByDate(msgs))
		printMessages(w, msgs)

		// Query user for message to print
		fmt.Fprintf(w, "Choose message [n]: ")
		msgIdx := readInt(rd)
		if msgIdx+1 > len(msgs) {
			fmt.Fprintf(w, "invalid message number\n")
			continue
		}
		printMsg(w, msgs[msgIdx])

		// Wait for OK
		fmt.Fprintf(w, "Reply (ctrl+c to quit) [y/N]: ")
		ans, _ := rd.ReadString('\n')
		if ans[0] == 'y' {
			composeMessage(msgs[msgIdx])
		}
	}
}

func readInt(rd *bufio.Reader) int {
	ans, _ := rd.ReadString('\n')
	i, _ := strconv.Atoi(strings.TrimSpace(ans))
	return i
}

type PrettyAddrSlice []wl2k.Address

func (addrs PrettyAddrSlice) String() string {
	var buf bytes.Buffer
	for i, addr := range addrs {
		fmt.Fprintf(&buf, "%s", addr.Addr)
		if i < len(addrs)-1 {
			fmt.Fprintf(&buf, ", ")
		}
	}
	return buf.String()
}

func printMsg(w io.Writer, msg *wl2k.Message) {
	fmt.Fprintf(w, "========================================\n")
	fmt.Fprintln(w, msg)
	fmt.Fprintf(w, "========================================\n\n")
}

func printMailboxes(w io.Writer) {
	for i, mailbox := range mailboxes {
		fmt.Fprintf(w, "%d:%s\t", i, mailbox)
	}
}

func printMessages(w io.Writer, msgs []*wl2k.Message) {
	rows := make([][]string, len(msgs))
	for i, msg := range msgs {
		to := msg.To[0].Addr
		if len(msg.To) > 1 {
			to = to + ", ..."
		}

		rows[i] = []string{
			fmt.Sprintf("%2d", i),
			msg.Subject,
			msg.From.Addr,
			msg.Date.String(),
			to,
		}
	}
	t := gotabulate.Create(rows)
	t.SetHeaders([]string{"i", "Subject", "From", "Date", "To"})
	t.SetAlign("left")
	t.SetWrapStrings(true)
	t.SetMaxCellSize(60)
	fmt.Fprintln(w, t.Render("simple"))
}
