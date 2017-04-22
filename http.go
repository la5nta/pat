// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/microcosm-cc/bluemonday"

	"github.com/la5nta/wl2k-go/catalog"
	"github.com/la5nta/wl2k-go/fbb"
	"github.com/la5nta/wl2k-go/mailbox"
)

type Progress struct {
	BytesTransferred int    `json:"bytes_transferred"`
	BytesTotal       int    `json:"bytes_total"`
	MID              string `json:"mid"`
	Subject          string `json:"subject"`
	Receiving        bool   `json:"receiving"`
	Sending          bool   `json:"sending"`
}

var webProgress Progress

//go:generate go install -v ./vendor/github.com/jteeuwen/go-bindata/go-bindata ./vendor/github.com/elazarl/go-bindata-assetfs/go-bindata-assetfs
//go:generate go-bindata-assetfs res/...
func ListenAndServe(addr string) error {
	log.Printf("Starting HTTP service (%s)...", addr)

	r := mux.NewRouter()
	r.HandleFunc("/api/connect_aliases", connectAliasesHandler).Methods("GET")
	r.HandleFunc("/api/connect", ConnectHandler)
	r.HandleFunc("/api/mailbox/{box}", mailboxHandler).Methods("GET")
	r.HandleFunc("/api/mailbox/{box}/{mid}", messageHandler).Methods("GET")
	r.HandleFunc("/api/mailbox/{box}/{mid}/{attachment}", attachmentHandler).Methods("GET")
	r.HandleFunc("/api/mailbox/{box}/{mid}/read", readHandler).Methods("POST")
	r.HandleFunc("/api/mailbox/out", postMessageHandler).Methods("POST")
	r.HandleFunc("/api/posreport", postPositionHandler).Methods("POST")
	r.HandleFunc("/api/status", statusHandler).Methods("GET")
	r.HandleFunc("/ws", wsHandler)
	r.HandleFunc("/ui", uiHandler).Methods("GET")
	r.HandleFunc("/", rootHandler).Methods("GET")

	http.Handle("/", r)
	http.Handle("/res/", http.StripPrefix("/res/", http.FileServer(assetFS())))

	return http.ListenAndServe(addr, nil)
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/ui", http.StatusFound)
}

func connectAliasesHandler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(config.ConnectAliases)
}

func readHandler(w http.ResponseWriter, r *http.Request) {
	var data struct{ Read bool }
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Printf("%s %s: %s", r.Method, r.URL.Path, err)
		return
	}

	box, mid := mux.Vars(r)["box"], mux.Vars(r)["mid"]

	msg, err := mailbox.OpenMessage(path.Join(mbox.MBoxPath, box, mid+mailbox.Ext))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := mailbox.SetUnread(msg, !data.Read); err != nil {
		log.Printf("%s %s: %s", r.Method, r.URL.Path, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func postPositionHandler(w http.ResponseWriter, r *http.Request) {
	var pos catalog.PosReport

	if err := json.NewDecoder(r.Body).Decode(&pos); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	r.Body.Close()

	// Post to outbox
	msg := pos.Message(fOptions.MyCall)
	if err := mbox.AddOut(msg); err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		fmt.Fprintln(w, "Position update posted")
	}
}

func postMessageHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 * (1024 ^ 2)) // 10Mb
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	m := r.MultipartForm

	msg := fbb.NewMessage(fbb.Private, fOptions.MyCall)

	// files
	files := m.File["files"]
	for _, f := range files {
		file, err := f.Open()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		p, err := ioutil.ReadAll(file)
		file.Close()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if isImageMediaType(f.Filename, f.Header.Get("Content-Type")) {
			log.Printf("Auto converting '%s' [%s]...", f.Filename, f.Header.Get("Content-Type"))

			if converted, err := convertImage(bytes.NewReader(p)); err != nil {
				log.Printf("Error converting image: %s", err)
			} else {
				log.Printf("Done converting '%s'.", f.Filename)

				ext := path.Ext(f.Filename)
				f.Filename = f.Filename[:len(f.Filename)-len(ext)] + ".jpg"
				p = converted
			}
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		msg.AddFile(fbb.NewFile(f.Filename, p))
	}

	// Other fields
	if v := m.Value["to"]; len(v) == 1 {
		addrs := strings.FieldsFunc(v[0], SplitFunc)
		msg.AddTo(addrs...)
	}
	if v := m.Value["subject"]; len(v) == 1 {
		msg.SetSubject(v[0])
	}
	if v := m.Value["body"]; len(v) == 1 {
		msg.SetBody(v[0])
	}
	if v := m.Value["p2ponly"]; len(v) == 1 && v[0] != "" {
		msg.Header.Set("X-P2POnly", "true")
	}
	if v := m.Value["date"]; len(v) == 1 {
		t, err := time.Parse(time.RFC3339, v[0])
		if err != nil {
			log.Printf("Unable to parse message date: %s", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		msg.SetDate(t)
	} else {
		log.Printf("Missing date value")
		http.Error(w, "Missing date value", http.StatusBadRequest)
		return
	}

	// Post to outbox
	if err := mbox.AddOut(msg); err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		var buf bytes.Buffer
		msg.Write(&buf)
		fmt.Fprintf(w, "Message posted (%.2f kB)", float64(buf.Len()/1024))
	}
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	go wsReadLoop(conn)
	defer conn.Close()

	lines, done, err := tailFile(fOptions.LogPath)
	if err != nil {
		log.Println(err)
		return
	}
	defer close(done)

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

	statusUpdateTick := time.Tick(200 * time.Millisecond)

	for {
		select {
		// Periodic status and progress update
		case <-statusUpdateTick:
			conn.WriteJSON(struct{ Progress Progress }{webProgress})
			err = conn.WriteJSON(struct{ Status statusUpdate }{getStatus()})

		// Log events
		case line := <-lines:
			err = conn.WriteJSON(struct {
				LogLine string
			}{string(line)})

		// Filsystem events
		case <-fsWatcher.Events:
			drainEvents(fsWatcher)
			err = conn.WriteJSON(struct {
				UpdateMailbox bool
			}{true})
		case err := <-fsWatcher.Errors:
			log.Println(err)
		}

		if err != nil {
			if err != websocket.ErrCloseSent {
				log.Println(err)
			}
			break
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

func wsReadLoop(c *websocket.Conn) {
	for {
		if _, _, err := c.NextReader(); err != nil {
			c.Close()
			break
		}
	}
}

func uiHandler(w http.ResponseWriter, r *http.Request) {
	data, err := Asset(path.Join("res", "tmpl", "index.html"))
	if err != nil {
		log.Fatal(err)
	}

	t := template.New("index.html") //create a new template
	t, err = t.Parse(string(data))
	if err != nil {
		log.Fatal(err)
	}

	tmplData := struct{ AppName, Version, Mycall, Addr string }{AppName, versionString(), fOptions.MyCall, r.Host}

	err = t.Execute(w, tmplData)
	if err != nil {
		log.Fatal(err)
	}
}

type statusUpdate struct {
	ActiveListeners []string `json:"active_listeners"`
	Connected       bool     `json:"connected"`
	RemoteAddr      string   `json:"remote_addr"`
}

func getStatus() statusUpdate {
	status := statusUpdate{
		ActiveListeners: make([]string, 0, len(listeners)),
		Connected:       exchangeConn != nil,
	}

	for method := range listeners {
		status.ActiveListeners = append(status.ActiveListeners, method)
	}
	sort.Strings(status.ActiveListeners)

	if exchangeConn != nil {
		addr := exchangeConn.RemoteAddr()
		status.RemoteAddr = fmt.Sprintf("%s:%s", addr.Network(), addr)
	}

	return status
}

func statusHandler(w http.ResponseWriter, req *http.Request) { json.NewEncoder(w).Encode(getStatus()) }

func ConnectHandler(w http.ResponseWriter, req *http.Request) {
	connectStr := req.FormValue("url")

	nMsgs := mbox.InboxCount()

	if success := Connect(connectStr); !success {
		http.Error(w, "Session failure", http.StatusInternalServerError)
	}

	json.NewEncoder(w).Encode(struct {
		NumReceived int
	}{
		mbox.InboxCount() - nMsgs,
	})
}

func mailboxHandler(w http.ResponseWriter, r *http.Request) {
	box := mux.Vars(r)["box"]

	var messages []*fbb.Message
	var err error

	switch box {
	case "in":
		messages, err = mbox.Inbox()
	case "out":
		messages, err = mbox.Outbox()
	case "sent":
		messages, err = mbox.Sent()
	case "archive":
		messages, err = mbox.Archive()
	default:
		http.NotFound(w, r)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Println(err)
	}

	sort.Sort(sort.Reverse(fbb.ByDate(messages)))

	jsonSlice := make([]JSONMessage, len(messages))
	for i, msg := range messages {
		jsonSlice[i] = JSONMessage{Message: msg}
	}
	json.NewEncoder(w).Encode(jsonSlice)
	return
}

type JSONMessage struct {
	*fbb.Message
	inclBody bool
}

func (m JSONMessage) MarshalJSON() ([]byte, error) {

	msg := struct {
		MID      string
		Date     time.Time
		From     fbb.Address
		To       []fbb.Address
		Cc       []fbb.Address
		Subject  string
		Body     string
		BodyHTML string
		Files    []*fbb.File
		P2POnly  bool
		Unread   bool
	}{
		MID:     m.MID(),
		Date:    m.Date(),
		From:    m.From(),
		To:      m.To(),
		Cc:      m.Cc(),
		Subject: m.Subject(),
		Files:   m.Files(),
		P2POnly: m.Header.Get("X-P2POnly") == "true",
		Unread:  mailbox.IsUnread(m.Message),
	}

	if m.inclBody {
		msg.Body, _ = m.Body()
		unsafe := toHTML([]byte(msg.Body))
		msg.BodyHTML = string(bluemonday.UGCPolicy().SanitizeBytes(unsafe))
	}
	return json.Marshal(msg)
}

func messageHandler(w http.ResponseWriter, r *http.Request) {
	box, mid := mux.Vars(r)["box"], mux.Vars(r)["mid"]

	msg, err := mailbox.OpenMessage(path.Join(mbox.MBoxPath, box, mid+mailbox.Ext))
	if os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(JSONMessage{msg, true})
}

func attachmentHandler(w http.ResponseWriter, r *http.Request) {
	box, mid, attachment := mux.Vars(r)["box"], mux.Vars(r)["mid"], mux.Vars(r)["attachment"]

	msg, err := mailbox.OpenMessage(path.Join(mbox.MBoxPath, box, mid+mailbox.Ext))
	if os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Find and write attachment
	var found bool
	for _, f := range msg.Files() {
		if f.Name() != attachment {
			continue
		}
		found = true
		http.ServeContent(w, r, f.Name(), msg.Date(), bytes.NewReader(f.Data()))
	}

	if !found {
		http.NotFound(w, r)
	}
}

// toHTML takes the given body and turns it into proper html with
// paragraphs, blockquote, and <br /> line breaks.
func toHTML(body []byte) []byte {
	buf := bytes.NewBuffer(body)
	var out bytes.Buffer

	fmt.Fprint(&out, "<p>")

	scanner := bufio.NewScanner(buf)

	var blockquote int
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			fmt.Fprint(&out, "</p><p>")
			continue
		}

		depth := blockquoteDepth(line)
		for depth != blockquote {
			if depth > blockquote {
				fmt.Fprintf(&out, "</p><blockquote><p>")
				blockquote++
			} else {
				fmt.Fprintf(&out, "</p></blockquote><p>")
				blockquote--
			}
		}
		line = line[depth:]

		line = htmlEncode(line)
		line = linkify(line)

		fmt.Fprint(&out, line+"\n")
	}

	for ; blockquote > 0; blockquote-- {
		fmt.Fprintf(&out, "</p></blockquote>")
	}

	fmt.Fprint(&out, "</p>")
	return out.Bytes()
}

// blcokquoteDepth counts the number of '>' at the beginning of the string.
func blockquoteDepth(str string) (n int) {
	for _, c := range str {
		if c != '>' {
			break
		}
		n++
	}
	return
}

// htmlEncode encodes html characters
func htmlEncode(str string) string {
	str = strings.Replace(str, ">", "&gt;", -1)
	str = strings.Replace(str, "<", "&lt;", -1)
	return str
}

// linkify detects url's in the given string and adds <a href tag.
//
// It is recursive.
func linkify(str string) string {
	start := strings.Index(str, "http")

	var needScheme bool
	if start < 0 {
		start = strings.Index(str, "www.")
		needScheme = true
	}

	if start < 0 {
		return str
	}

	end := strings.IndexAny(str[start:], " ,()[]")
	if end < 0 {
		end = len(str)
	} else {
		end += start
	}

	link := str[start:end]
	if needScheme {
		link = "http://" + link
	}

	return fmt.Sprintf(`%s<a href='%s' target='_blank'>%s</a>%s`, str[:start], link, str[start:end], linkify(str[end:]))
}
