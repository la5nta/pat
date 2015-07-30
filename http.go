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

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"

	"github.com/la5nta/wl2k-go"
	"github.com/la5nta/wl2k-go/catalog"
)

//go:generate go-bindata-assetfs res/...
func ListenAndServe(addr string) error {
	log.Printf("Starting HTTP service (%s)...", addr)

	r := mux.NewRouter()
	r.HandleFunc("/api/connect/{method}", ConnectHandler)
	r.HandleFunc("/api/listen", ListenHandler)
	r.HandleFunc("/api/mailbox/{box}", mailboxHandler).Methods("GET")
	r.HandleFunc("/api/mailbox/{box}/{mid}", messageHandler).Methods("GET")
	r.HandleFunc("/api/mailbox/{box}/{mid}/{attachment}", attachmentHandler).Methods("GET")
	r.HandleFunc("/api/mailbox/out", postMessageHandler).Methods("POST")
	r.HandleFunc("/api/posreport", postPositionHandler).Methods("POST")
	r.HandleFunc("/api/status", statusHandler).Methods("GET")
	r.HandleFunc("/ws", consoleHandler)
	r.HandleFunc("/ui", uiHandler).Methods("GET")
	r.HandleFunc("/", rootHandler).Methods("GET")

	http.Handle("/", r)
	http.Handle("/res/", http.StripPrefix("/res/", http.FileServer(assetFS())))

	return http.ListenAndServe(addr, nil)
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/ui", http.StatusFound)
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

	msg := wl2k.NewMessage(wl2k.Private, fOptions.MyCall)

	// files
	files := m.File["files"]
	for _, f := range files {
		file, err := f.Open()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var p []byte
		if isImageMediaType(f.Filename, f.Header.Get("Content-Type")) {
			log.Printf("Auto converting '%s' [%s]...", f.Filename, f.Header.Get("Content-Type"))
			p, err = convertImage(file)
			if err != nil {
				log.Printf("Error converting image: %s", err)
			} else {
				f.Filename += ".JPG"
				log.Printf("Done converting '%s'.", f.Filename)
			}
		}

		if p == nil || err != nil {
			p, err = ioutil.ReadAll(file)
		}

		file.Close()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		msg.AddFile(wl2k.NewFile(f.Filename, p))
	}

	// Other fields
	if v := m.Value["to"]; len(v) == 1 {
		addrs := strings.Split(v[0], ",")
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

func consoleHandler(w http.ResponseWriter, r *http.Request) {
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

	for line := range lines {
		err = conn.WriteMessage(websocket.TextMessage, append(line, '\n'))
		if err != nil {
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

	tmplData := struct{ AppName, Mycall, Addr string }{"wl2k-go", fOptions.MyCall, r.Host}

	err = t.Execute(w, tmplData)
	if err != nil {
		log.Fatal(err)
	}
}

func statusHandler(w http.ResponseWriter, req *http.Request) {
	status := struct {
		ActiveListeners []string `json:"active_listeners"`
		Connected       bool     `json:"connected"`
		RemoteAddr      string   `json:"remote_addr"`
	}{
		ActiveListeners: make([]string, 0, len(listeners)),
		Connected:       exchangeConn != nil,
	}

	for method, _ := range listeners {
		status.ActiveListeners = append(status.ActiveListeners, method)
	}
	sort.Strings(status.ActiveListeners)

	if exchangeConn != nil {
		addr := exchangeConn.RemoteAddr()
		status.RemoteAddr = fmt.Sprintf("%s:%s", addr.Network(), addr)
	}

	json.NewEncoder(w).Encode(status)
	return
}

func ConnectHandler(w http.ResponseWriter, req *http.Request) {
	connectStr := mux.Vars(req)["method"]

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

func ListenHandler(w http.ResponseWriter, req *http.Request) {
	listenStr := path.Base(req.RequestURI)
	Listen(listenStr)
	/*if err := Listen(listenStr); err != nil {
		http.Error(w, "Listen failure", http.StatusInternalServerError)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Println(err)
	}*/
}

func mailboxHandler(w http.ResponseWriter, r *http.Request) {
	box := mux.Vars(r)["box"]

	var messages []*wl2k.Message
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

	sort.Sort(sort.Reverse(wl2k.ByDate(messages)))

	jsonSlice := make([]JSONMessage, len(messages))
	for i, msg := range messages {
		jsonSlice[i] = JSONMessage{msg}
	}
	json.NewEncoder(w).Encode(jsonSlice)

	return
}

type JSONMessage struct{ *wl2k.Message }

func (m JSONMessage) MarshalJSON() ([]byte, error) {
	body, _ := m.Body()
	msg := struct {
		MID     string
		Date    time.Time
		From    wl2k.Address
		To      []wl2k.Address
		Cc      []wl2k.Address
		Subject string
		Body    string
		Files   []*wl2k.File
		P2POnly bool
	}{
		m.MID(), m.Date(), m.From(), m.To(), m.Cc(), m.Subject(), body, m.Files(), m.Header.Get("X-P2POnly") == "true",
	}

	return json.Marshal(msg)
}

func messageHandler(w http.ResponseWriter, r *http.Request) {
	box, mid := mux.Vars(r)["box"], mux.Vars(r)["mid"]

	file, err := os.Open(path.Join(mbox.MBoxPath, box, mid))
	if os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	msg := new(wl2k.Message)
	if err := msg.ReadFrom(file); err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(JSONMessage{msg})
}

func attachmentHandler(w http.ResponseWriter, r *http.Request) {
	box, mid, attachment := mux.Vars(r)["box"], mux.Vars(r)["mid"], mux.Vars(r)["attachment"]

	file, err := os.Open(path.Join(mbox.MBoxPath, box, mid))
	if os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	msg := new(wl2k.Message)
	if err := msg.ReadFrom(file); err != nil {
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
