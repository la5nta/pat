// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// Package mailbox provides mailbox handlers for a wl2k.Session.
package mailbox

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path"

	"github.com/la5nta/wl2k-go"
)

const (
	DIR_INBOX   = "/in/"
	DIR_OUTBOX  = "/out/"
	DIR_SENT    = "/sent/"
	DIR_ARCHIVE = "/archive/"
)

// NewDirHandler is a file system (directory) oriented mailbox handler.
type DirHandler struct {
	MBoxPath string
	deferred map[string]bool
	sendOnly bool
}

// NewDirHandler wraps the directory given by path as a DirHandler.
//
// If sendOnly is true, all inbound messages will be deferred.
func NewDirHandler(path string, sendOnly bool) *DirHandler {
	return &DirHandler{
		MBoxPath: path,
		sendOnly: sendOnly,
	}
}

func (h *DirHandler) Prepare() (err error) {
	h.deferred = make(map[string]bool)
	return ensureDirStructure(h.MBoxPath)
}

func (h *DirHandler) Inbox() ([]*wl2k.Message, error) {
	return LoadMessageDir(path.Join(h.MBoxPath, DIR_INBOX))
}

func (h *DirHandler) Outbox() ([]*wl2k.Message, error) {
	return LoadMessageDir(path.Join(h.MBoxPath, DIR_OUTBOX))
}

func (h *DirHandler) Sent() ([]*wl2k.Message, error) {
	return LoadMessageDir(path.Join(h.MBoxPath, DIR_SENT))
}

func (h *DirHandler) Archive() ([]*wl2k.Message, error) {
	return LoadMessageDir(path.Join(h.MBoxPath, DIR_ARCHIVE))
}

// InboxCount returns the number of messages in the inbox. -1 on error.
func (h *DirHandler) InboxCount() int   { return countFiles(path.Join(h.MBoxPath, DIR_INBOX)) }
func (h *DirHandler) OutboxCount() int  { return countFiles(path.Join(h.MBoxPath, DIR_OUTBOX)) }
func (h *DirHandler) SentCount() int    { return countFiles(path.Join(h.MBoxPath, DIR_SENT)) }
func (h *DirHandler) ArchiveCount() int { return countFiles(path.Join(h.MBoxPath, DIR_ARCHIVE)) }

func (h *DirHandler) AddOut(msg *wl2k.Message) error {
	data, err := msg.Bytes()
	if err != nil {
		return err
	}

	return ioutil.WriteFile(path.Join(h.MBoxPath, DIR_OUTBOX, msg.MID()), data, 0644)
}

func (h *DirHandler) ProcessInbound(msgs ...*wl2k.Message) (err error) {
	dir := path.Join(h.MBoxPath, DIR_INBOX)
	for _, m := range msgs {
		filename := path.Join(dir, m.MID())

		m.Header.Set("X-Unread", "true")

		data, err := m.Bytes()
		if err != nil {
			return err
		}

		if err = ioutil.WriteFile(filename, data, 0664); err != nil {
			err = fmt.Errorf("Unable to write received message (%s): %s", filename, err)
		}
	}
	return
}

func (h *DirHandler) GetInboundAnswer(p wl2k.Proposal) wl2k.ProposalAnswer {
	if h.sendOnly {
		return wl2k.Defer
	}

	// Check if file exists
	f, err := os.Open(path.Join(h.MBoxPath, DIR_INBOX, p.MID()))
	if err == nil {
		f.Close()
		return wl2k.Reject
	} else if os.IsNotExist(err) {
		return wl2k.Accept
	} else if err != nil {
		log.Printf("Unable to determin if %s has been received: %s", p.MID(), err)
	}

	return wl2k.Accept
}

func (h *DirHandler) SetSent(MID string, rejected bool) {
	oldPath := path.Join(h.MBoxPath, DIR_OUTBOX, MID)
	newPath := path.Join(h.MBoxPath, DIR_SENT, MID)

	if err := os.Rename(oldPath, newPath); err != nil {
		log.Fatalf("Unable to move %s to %s: %s", oldPath, newPath, err)
	}
}

func (h *DirHandler) SetDeferred(MID string) {
	h.deferred[MID] = true
}

func (h *DirHandler) GetOutbound(fws ...wl2k.Address) []*wl2k.Message {
	all, err := LoadMessageDir(path.Join(h.MBoxPath, DIR_OUTBOX))
	if err != nil {
		log.Println(err)
	}

	deliver := make([]*wl2k.Message, 0, len(all))
	for _, m := range all {
		if h.deferred[m.MID()] {
			continue
		}

		// Check unsent messages that are addressed to one of the
		// forwarder addresses of the remote.
		if len(fws) > 0 {
			for _, fw := range fws {
				if m.IsOnlyReceiver(fw) {
					deliver = append(deliver, m)
					break
				}
			}
			continue
		}

		if len(fws) == 0 && m.Header.Get("X-P2POnly") == "true" {
			continue // The message is P2POnly and remote is CMS
		}

		// Remove private headers
		m.Header.Del("X-P2POnly")
		m.Header.Del("X-FilePath")
		m.Header.Del("X-Unread")

		deliver = append(deliver, m)
	}
	return deliver
}

func DefaultMailboxPath() (string, error) {
	appdir, err := DefaultAppDir()
	if err != nil {
		return "", fmt.Errorf("Unable to determine application directory: %s", err)
	}
	return path.Join(appdir, "mailbox"), nil
}

func DefaultAppDir() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("Unable to determine home directory: %s", err)
	}
	return path.Join(usr.HomeDir, ".wl2k"), nil
}

func ensureDirStructure(mboxPath string) (err error) {
	mode := os.ModeDir | os.ModePerm
	if err = os.MkdirAll(path.Join(mboxPath, DIR_INBOX), mode); err != nil {
		return
	} else if err = os.MkdirAll(path.Join(mboxPath, DIR_OUTBOX), mode); err != nil {
		return
	} else if err = os.MkdirAll(path.Join(mboxPath, DIR_SENT), mode); err != nil {
		return
	} else if err = os.MkdirAll(path.Join(mboxPath, DIR_ARCHIVE), mode); err != nil {
		return
	}
	return
}

func UserPath(root, callsign string) string {
	return path.Join(root, callsign)
}

func countFiles(dirPath string) int {
	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return -1
	}

	return len(files)
}

func LoadMessageDir(dirPath string) ([]*wl2k.Message, error) {
	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("Unable to read dir (%s): %s", dirPath, err)
	}

	msgs := make([]*wl2k.Message, 0, len(files))

	for _, file := range files {
		if file.IsDir() || file.Name()[0] == '.' {
			continue
		}

		msg, err := OpenMessage(path.Join(dirPath, file.Name()))
		if err != nil {
			return nil, err
		}

		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// OpenMessage opens a single a wl2k.Message file.
func OpenMessage(path string) (*wl2k.Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("Unable to open file (%s): %s", path, err)
	}
	defer f.Close()

	message := new(wl2k.Message)
	if err := message.ReadFrom(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("Unable to parse message (%s): %s", path, err)
	}

	message.Header.Set("X-FilePath", path)
	return message, nil
}

// IsUnread returns true if the given message is marked as unread.
func IsUnread(msg *wl2k.Message) bool { return msg.Header.Get("X-Unread") == "true" }

// SetUnread marks the given message as read/unread and re-writes the file to disk.
func SetUnread(msg *wl2k.Message, unread bool) error {
	if !unread && msg.Header.Get("X-Unread") == "" {
		return nil
	}

	if unread {
		msg.Header.Set("X-Unread", "true")
	} else {
		msg.Header.Del("X-Unread")
	}

	data, err := msg.Bytes()
	if err != nil {
		return err
	}

	filePath := msg.Header.Get("X-FilePath")
	if filePath == "" {
		return fmt.Errorf("Missing X-FilePath header")
	}
	return ioutil.WriteFile(filePath, data, 0644)
}
