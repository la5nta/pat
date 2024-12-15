// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/bndr/gotabulate"

	"github.com/la5nta/wl2k-go/fbb"
	"github.com/la5nta/wl2k-go/mailbox"
)

var mailboxes = []string{"in", "out", "sent", "archive"}

func readMail(ctx context.Context) {
	w := os.Stdout

	for {
		// Query user for mailbox to list
		printMailboxes(w)
		fmt.Fprintf(w, "\nChoose mailbox [n]: ")
		mailboxIdx, ok := readInt(ctx)
		if !ok {
			break
		} else if mailboxIdx+1 > len(mailboxes) {
			fmt.Fprintln(w, "Invalid mailbox number")
			continue
		}

		for {
			// Fetch messages
			msgs, err := mailbox.LoadMessageDir(filepath.Join(mbox.MBoxPath, mailboxes[mailboxIdx]))
			if err != nil {
				log.Fatal(err)
			} else if len(msgs) == 0 {
				fmt.Fprintf(w, "(empty)\n")
				break
			}

			// Print messages (sorted by date)
			sort.Sort(fbb.ByDate(msgs))
			printMessages(w, msgs)

			// Query user for message to print
			fmt.Fprintf(w, "Choose message [n]: ")
			msgIdx, ok := readInt(ctx)
			if !ok {
				break
			} else if msgIdx+1 > len(msgs) {
				fmt.Fprintf(w, "invalid message number\n")
				continue
			}
			printMsg(w, msgs[msgIdx])

			// Mark as read?
			if mailbox.IsUnread(msgs[msgIdx]) {
				fmt.Fprintf(w, "Mark as read? [Y/n]: ")
				ans := readLine()
				if ans == "" || strings.EqualFold(ans, "y") {
					mailbox.SetUnread(msgs[msgIdx], false)
				}
			}

		L:
			for {
				fmt.Fprintf(w, "Action [C,r,e,q,?]: ")
				switch readLine() {
				case "C", "c", "":
					break L
				case "r":
					composeReplyMessage(msgs[msgIdx])
				case "e":
					extractMessageHandle(ctx, []string{msgs[msgIdx].MID()})
				case "q":
					return
				case "?":
					fallthrough
				default:
					fmt.Fprintln(w, "c - continue")
					fmt.Fprintln(w, "r - reply")
					fmt.Fprintln(w, "e - extract (attachments)")
					fmt.Fprintln(w, "q - quit")
				}
			}
		}
	}
}

func readInt(ctx context.Context) (int, bool) {
	cs := make(chan string, 1)
	go func() { cs <- readLine() }()
	select {
	case <-ctx.Done():
		return 0, false
	case str := <-cs:
		if str == "" {
			return 0, false
		}
		i, _ := strconv.Atoi(str)
		return i, true
	}
}

type PrettyAddrSlice []fbb.Address

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

func printMsg(w io.Writer, msg *fbb.Message) {
	fmt.Fprintf(w, "========================================\n")
	fmt.Fprintln(w, msg)
	fmt.Fprintf(w, "========================================\n\n")
}

func printMailboxes(w io.Writer) {
	for i, mbox := range mailboxes {
		fmt.Fprintf(w, "%d:%s\t", i, mbox)
	}
}

func printMessages(w io.Writer, msgs []*fbb.Message) {
	rows := make([][]string, len(msgs))
	for i, msg := range msgs {
		var to string
		if len(msg.To()) > 0 {
			to = msg.To()[0].Addr
		}
		if len(msg.To()) > 1 {
			to += ", ..."
		}

		var flags string
		if mailbox.IsUnread(msg) {
			flags += "N" // New
		}

		rows[i] = []string{
			fmt.Sprintf("%2d", i),
			flags,
			msg.Subject(),
			msg.From().Addr,
			msg.Date().String(),
			to,
		}
	}
	t := gotabulate.Create(rows)
	t.SetHeaders([]string{"i", "Flags", "Subject", "From", "Date", "To"})
	t.SetAlign("left")
	t.SetWrapStrings(true)
	t.SetMaxCellSize(60)
	fmt.Fprintln(w, t.Render("simple"))
}
