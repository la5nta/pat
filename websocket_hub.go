// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"os"
	"path"
	"runtime"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"

	"github.com/la5nta/pat/internal/osutil"
	"github.com/la5nta/wl2k-go/mailbox"
)

const KeepaliveInterval = 4 * time.Minute

// WSConn represent one connection in the WSHub pool
type WSConn struct {
	conn *websocket.Conn
	out  chan interface{}
}

// WSHub is a hub for broadcasting data to several websocket connections
type WSHub struct {
	mu   sync.Mutex
	pool map[*WSConn]struct{}
}

func NewWSHub() *WSHub {
	w := &WSHub{pool: map[*WSConn]struct{}{}}
	go w.watchMBox()
	return w
}

func (w *WSHub) UpdateStatus()            { w.WriteJSON(struct{ Status Status }{getStatus()}) }
func (w *WSHub) WriteProgress(p Progress) { w.WriteJSON(struct{ Progress Progress }{p}) }
func (w *WSHub) WriteNotification(n Notification) {
	w.WriteJSON(struct{ Notification Notification }{n})
}

func (w *WSHub) Prompt(p Prompt) {
	w.WriteJSON(struct{ Prompt Prompt }{p})
	go func() { <-p.cancel; w.WriteJSON(struct{ PromptAbort Prompt }{p}) }()
}

func (w *WSHub) WriteJSON(v interface{}) {
	if w == nil {
		return
	}

	w.mu.Lock()
	for c, _ := range w.pool {
		select {
		case c.out <- v:
		case <-time.After(3 * time.Second):
			log.Println("Closing one unresponsive web socket")
			c.conn.Close()
			delete(w.pool, c)
		}
	}
	w.mu.Unlock()
}

func (w *WSHub) ClientAddrs() []string {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	addrs := make([]string, 0, len(w.pool))
	for c, _ := range w.pool {
		addrs = append(addrs, c.conn.RemoteAddr().String())
	}
	return addrs
}

func (w *WSHub) watchMBox() {
	// Maximise ulimit -n:
	//   fsnotify opens a file descriptor for every file in the directories it watches, which
	//   may more files than the current soft limit. The is especially a problem on macOS which
	//   has a default soft limit of only 256 files. Windows does not have a such a limit.
	if runtime.GOOS != "windows" {
		if err := osutil.RaiseOpenFileLimit(4096); err != nil {
			log.Printf("Unable to raise open file limit: %v", err)
		}
	}

	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Println("Unable to start fs watcher: ", err)
	} else {
		p := path.Join(mbox.MBoxPath, mailbox.DIR_INBOX)
		if err := fsWatcher.Add(p); err != nil {
			log.Printf("Unable to add path '%s' to fs watcher: %s", p, err)
		}

		// These will probably fail if the first failed, but it's not important to log all.
		fsWatcher.Add(path.Join(mbox.MBoxPath, mailbox.DIR_OUTBOX))
		fsWatcher.Add(path.Join(mbox.MBoxPath, mailbox.DIR_SENT))
		fsWatcher.Add(path.Join(mbox.MBoxPath, mailbox.DIR_ARCHIVE))
		defer fsWatcher.Close()
	}

	for {
		select {
		// Filesystem events
		case <-fsWatcher.Events:
			drainEvents(fsWatcher)
			websocketHub.WriteJSON(struct {
				UpdateMailbox bool
			}{true})
		case err := <-fsWatcher.Errors:
			log.Println(err)
		}
	}
}

// Handle adds a new websocket to the hub
//
// It will block until the client either stops responding or closes the connection.
func (w *WSHub) Handle(conn *websocket.Conn) {
	c := &WSConn{
		conn: conn,
		out:  make(chan interface{}, 1),
	}

	w.mu.Lock()
	w.pool[c] = struct{}{}
	w.mu.Unlock()

	// Initial status update
	// (broadcasted as it includes info to other clients about this new one)
	w.UpdateStatus()

	quit := wsReadLoop(conn)

	lines, done, err := tailFile(fOptions.LogPath)
	if err != nil {
		log.Println(err)
		return
	}
	defer close(done)

	for {
		select {
		case <-time.After(KeepaliveInterval):
			c.conn.WriteJSON(struct {
				Ping bool
			}{true})
		case line := <-lines:
			c.conn.WriteJSON(struct {
				LogLine string
			}{string(line)})
		case v := <-c.out:
			err := c.conn.WriteJSON(v)
			if err != nil {
				log.Println(err)
			}
		case <-quit:
			// The read loop failed/disconnected. Remove from hub.
			c.conn.Close()
			w.mu.Lock()
			delete(w.pool, c)
			defer w.UpdateStatus()
			w.mu.Unlock()
			return
		}
	}
}

func drainEvents(w *fsnotify.Watcher) {
	for {
		select {
		case <-w.Events:
		default:
			return
		}
	}
}

// Expects the file to never get renamed/truncated or deleted
func tailFile(path string) (<-chan []byte, chan<- struct{}, error) {
	lines := make(chan []byte)
	done := make(chan struct{})
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}

	go func() {
		rd := bufio.NewReader(file)
		for {
			data, _, err := rd.ReadLine()
			if err == io.EOF {
				time.Sleep(time.Millisecond * 100)
				continue
			}

			select {
			case <-done:
				file.Close()
				return
			case lines <- data:
			}
		}
	}()

	return (<-chan []byte)(lines), (chan<- struct{})(done), nil
}

func handleWSMessage(v map[string]json.RawMessage) {
	raw, ok := v["prompt_response"]
	if !ok {
		return
	}
	var resp PromptResponse
	json.Unmarshal(raw, &resp)
	promptHub.Respond(resp.ID, resp.Value, resp.Err)
}

func wsReadLoop(c *websocket.Conn) <-chan struct{} {
	quit := make(chan struct{})
	go func() {
		for {
			v := map[string]json.RawMessage{}
			err := c.ReadJSON(&v)
			if err != nil {
				close(quit)
				return
			}
			if _, ok := v["Pong"]; ok {
				// For Keepalive only. Could use a delayed ping response to detect dead connections in the future.
				continue
			}
			go handleWSMessage(v)
		}
	}()
	return quit
}
