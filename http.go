// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/la5nta/pat/internal/buildinfo"
	"github.com/la5nta/pat/internal/gpsd"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/la5nta/wl2k-go/catalog"
	"github.com/la5nta/wl2k-go/fbb"
	"github.com/la5nta/wl2k-go/mailbox"
	"github.com/microcosm-cc/bluemonday"
)

//go:embed web/res/**
var embeddedFS embed.FS

var ErrNotFound = HTTPError{errors.New("Not found"), http.StatusNotFound}

// Status represents a status report as sent to the Web GUI
type Status struct {
	ActiveListeners []string `json:"active_listeners"`
	Connected       bool     `json:"connected"`
	Dialing         bool     `json:"dialing"`
	RemoteAddr      string   `json:"remote_addr"`
	HTTPClients     []string `json:"http_clients"`
}

// Progress represents a progress report as sent to the Web GUI
type Progress struct {
	BytesTransferred int    `json:"bytes_transferred"`
	BytesTotal       int    `json:"bytes_total"`
	MID              string `json:"mid"`
	Subject          string `json:"subject"`
	Receiving        bool   `json:"receiving"`
	Sending          bool   `json:"sending"`
	Done             bool   `json:"done"`
}

// Notification represents a desktop notification as sent to the Web GUI
type Notification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type HTTPError struct {
	error
	StatusCode int
}

type JSONHandlerFunc func(w http.ResponseWriter, req *http.Request) (interface{}, error)

func (h JSONHandlerFunc) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, err := h(w, req)
	if resp == nil && err == nil {
		return
	}
	switch err := err.(type) {
	case nil:
		_ = json.NewEncoder(w).Encode(resp)
	case HTTPError:
		http.Error(w, err.Error(), err.StatusCode)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var websocketHub *WSHub

type Router struct{ *mux.Router }

func (r Router) HandleJSON(path string, h JSONHandlerFunc) *mux.Route {
	return r.Handle(path, h)
}

func ListenAndServe(addr string) error {
	log.Printf("Starting HTTP service (http://%s)...", addr)

	if host, _, _ := net.SplitHostPort(addr); host == "" && config.GPSd.EnableHTTP {
		// TODO: maybe make a popup showing the warning ont the web UI?
		_, _ = fmt.Fprintf(logWriter, "\nWARNING: You have enable GPSd HTTP endpoint (enable_http). You might expose"+
			"\n         your current position to anyone who has access to the Pat web interface!\n\n")
	}

	websocketHub = NewWSHub()

	r := Router{mux.NewRouter()}

	// API endpoints
	r.HandleJSON("/api/connect_aliases", connectAliasesHandler).Methods("GET")
	r.HandleJSON("/api/connect", ConnectHandler)
	r.HandleFunc("/api/formcatalog", formsMgr.GetFormsCatalogHandler).Methods("GET")
	r.HandleFunc("/api/form", formsMgr.PostFormDataHandler).Methods("POST")
	r.HandleFunc("/api/form", formsMgr.GetFormDataHandler).Methods("GET")
	r.HandleFunc("/api/forms", formsMgr.GetFormTemplateHandler).Methods("GET")
	r.HandleFunc("/api/formsUpdate", formsMgr.UpdateFormTemplatesHandler).Methods("POST")
	r.HandleJSON("/api/disconnect", DisconnectHandler)
	r.HandleJSON("/api/mailbox/{box}", mailboxHandler).Methods("GET")
	r.HandleJSON("/api/mailbox/{box}/{mid}", messageHandler).Methods("GET")
	r.HandleJSON("/api/mailbox/{box}/{mid}", messageDeleteHandler).Methods("DELETE")
	r.HandleFunc("/api/mailbox/{box}/{mid}/{attachment}", attachmentHandler).Methods("GET")
	r.HandleJSON("/api/mailbox/{box}/{mid}/read", readHandler).Methods("POST")
	r.HandleJSON("/api/mailbox/{box}", postMessageHandler).Methods("POST")
	r.HandleJSON("/api/posreport", postPositionHandler).Methods("POST")
	r.HandleJSON("/api/status", statusHandler).Methods("GET")
	r.HandleJSON("/api/current_gps_position", positionHandler).Methods("GET")
	r.HandleJSON("/api/qsy", qsyHandler).Methods("POST")
	r.HandleJSON("/api/rmslist", rmslistHandler).Methods("GET")

	// Websocket handler
	r.HandleFunc("/ws", wsHandler)

	// Web GUI assets
	{
		staticContent, err := fs.Sub(embeddedFS, "web")
		if err != nil {
			return err
		}
		r.HandleFunc("/ui", uiHandler(staticContent)).Methods("GET")
		r.PathPrefix("/res/").Handler(http.FileServer(http.FS(staticContent)))
		r.Handle("/", http.RedirectHandler("/ui", http.StatusFound))
	}

	server := http.Server{
		Addr:    addr,
		Handler: r,
	}
	return server.ListenAndServe()
}

func connectAliasesHandler(_ http.ResponseWriter, _ *http.Request) (interface{}, error) {
	return config.ConnectAliases, nil
}

func readHandler(_ http.ResponseWriter, r *http.Request) (interface{}, error) {
	var data struct{ Read bool }
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		log.Printf("%s %s: %s", r.Method, r.URL.Path, err)
		return nil, HTTPError{err, http.StatusBadRequest}
	}

	box, mid := mux.Vars(r)["box"], mux.Vars(r)["mid"]

	msg, err := mailbox.OpenMessage(path.Join(mbox.MBoxPath, box, mid+mailbox.Ext))
	if err != nil {
		return nil, err
	}

	if err := mailbox.SetUnread(msg, !data.Read); err != nil {
		log.Printf("%s %s: %s", r.Method, r.URL.Path, err)
		return nil, err
	}
	return nil, nil
}

func postPositionHandler(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	var pos catalog.PosReport

	if err := json.NewDecoder(r.Body).Decode(&pos); err != nil {
		return nil, HTTPError{err, http.StatusBadRequest}
	}

	if pos.Date.IsZero() {
		pos.Date = time.Now()
	}

	// Post to outbox
	msg := pos.Message(fOptions.MyCall)
	if err := mbox.AddOut(msg); err != nil {
		log.Println(err)
		return nil, err
	}
	_, _ = fmt.Fprintln(w, "Position update posted")
	return nil, nil
}

func isInPath(base string, path string) error {
	_, err := filepath.Rel(base, path)
	return err
}

func postMessageHandler(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	box := mux.Vars(r)["box"]
	if box == "out" {
		return postOutboundMessageHandler(w, r)
	}

	srcPath := r.Header.Get("X-Pat-SourcePath")
	if srcPath == "" {
		return nil, HTTPError{errors.New("not implemented"), http.StatusNotImplemented}
	}

	srcPath = strings.TrimPrefix(srcPath, "/api/mailbox/")
	srcPath = filepath.Join(mbox.MBoxPath, srcPath+mailbox.Ext)

	// Check that we don't escape our mailbox path
	srcPath = filepath.Clean(srcPath)
	if err := isInPath(mbox.MBoxPath, srcPath); err != nil {
		err = fmt.Errorf("malicious source path in move: %w", err)
		return nil, HTTPError{err, http.StatusBadRequest}
	}

	targetPath := filepath.Join(mbox.MBoxPath, box, filepath.Base(srcPath))

	if err := os.Rename(srcPath, targetPath); err != nil {
		err = fmt.Errorf("failed to move message: %w", err)
		return nil, HTTPError{err, http.StatusBadRequest}
	}
	return "OK", nil
}

func postOutboundMessageHandler(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	err := r.ParseMultipartForm(10 * (1024 ^ 2)) // 10Mb
	if err != nil {
		if err := r.ParseForm(); err != nil {
			return nil, err
		}
	}
	msg := fbb.NewMessage(fbb.Private, fOptions.MyCall)

	// files
	if r.MultipartForm != nil {
		files := r.MultipartForm.File["files"]
		for _, f := range files {
			err := attachFile(f, msg)
			switch err := err.(type) {
			case nil:
				// No problem
			case HTTPError:
				http.Error(w, err.Error(), err.StatusCode)
			default:
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}
	}

	cookie, err := r.Cookie("forminstance")
	if err == nil {
		formData := formsMgr.GetPostedFormData(cookie.Value)
		name := formsMgr.GetXMLAttachmentNameForForm(formData.TargetForm, formData.IsReply)
		msg.AddFile(fbb.NewFile(name, []byte(formData.MsgXML)))
	}

	// Other fields
	if v := r.Form["to"]; len(v) == 1 {
		addrs := strings.FieldsFunc(v[0], SplitFunc)
		msg.AddTo(addrs...)
	}
	if v := r.Form["cc"]; len(v) == 1 {
		addrs := strings.FieldsFunc(v[0], SplitFunc)
		msg.AddCc(addrs...)
	}
	if v := r.Form["subject"]; len(v) == 1 {
		msg.SetSubject(v[0])
	}
	if v := r.Form["body"]; len(v) == 1 {
		_ = msg.SetBody(v[0])
	}
	if v := r.Form["p2ponly"]; len(v) == 1 && v[0] != "" {
		msg.Header.Set("X-P2POnly", "true")
	}
	if v := r.Form["date"]; len(v) == 1 {
		t, err := time.Parse(time.RFC3339, v[0])
		if err != nil {
			err = fmt.Errorf("unable to parse message date: %w", err)
			log.Println(err)
			return nil, HTTPError{err, http.StatusBadRequest}
		}
		msg.SetDate(t)
	} else {
		err := errors.New("missing date value")
		return nil, HTTPError{err, http.StatusBadRequest}
	}

	if err := msg.Validate(); err != nil {
		err = fmt.Errorf("validation error: %w", err)
		return nil, HTTPError{err, http.StatusBadRequest}
	}

	// Post to outbox
	if err := mbox.AddOut(msg); err != nil {
		log.Println(err)
		return nil, err
	}

	w.WriteHeader(http.StatusCreated)
	var buf bytes.Buffer
	_ = msg.Write(&buf)
	_, _ = fmt.Fprintf(w, "Message posted (%.2f kB)", float64(buf.Len()/1024))
	return nil, nil
}

func attachFile(f *multipart.FileHeader, msg *fbb.Message) error {
	// For some unknown reason, we receive this empty unnamed file when no
	// attachment is provided. Prior to Go 1.10, this was filtered by
	// multipart.Reader.
	if isEmptyFormFile(f) {
		return nil
	}

	if f.Filename == "" {
		err := errors.New("missing attachment name")
		return HTTPError{err, http.StatusBadRequest}
	}
	file, err := f.Open()
	if err != nil {
		return HTTPError{err, http.StatusInternalServerError}
	}

	p, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil {
		return HTTPError{err, http.StatusInternalServerError}
	}

	if isImageMediaType(f.Filename, f.Header.Get("Content-Type")) {
		log.Printf("Auto converting '%s' [%s]...", f.Filename, f.Header.Get("Content-Type"))

		if converted, err := convertImage(p); err != nil {
			log.Printf("Error converting image: %s", err)
		} else {
			log.Printf("Done converting '%s'.", f.Filename)

			ext := path.Ext(f.Filename)
			f.Filename = f.Filename[:len(f.Filename)-len(ext)] + ".jpg"
			p = converted
		}
	}

	msg.AddFile(fbb.NewFile(f.Filename, p))
	return nil
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
	_ = conn.WriteJSON(struct{ MyCall string }{fOptions.MyCall})
	websocketHub.Handle(conn)
}

func uiHandler(staticContent fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		data, err := fs.ReadFile(staticContent, path.Join("res", "tmpl", "index.html"))
		if err != nil {
			log.Fatal(err)
		}

		t := template.New("index.html") // create a new template
		t, err = t.Parse(string(data))
		if err != nil {
			log.Fatal(err)
		}

		tmplData := struct{ AppName, Version, Mycall string }{buildinfo.AppName, buildinfo.VersionString(), fOptions.MyCall}

		err = t.Execute(w, tmplData)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func getStatus() Status {
	status := Status{
		ActiveListeners: []string{},
		Dialing:         dialing != nil,
		Connected:       exchangeConn != nil,
		HTTPClients:     websocketHub.ClientAddrs(),
	}

	for _, tl := range listenHub.Active() {
		status.ActiveListeners = append(status.ActiveListeners, tl.Name())
	}
	sort.Strings(status.ActiveListeners)

	if exchangeConn != nil {
		addr := exchangeConn.RemoteAddr()
		status.RemoteAddr = fmt.Sprintf("%s:%s", addr.Network(), addr)
	}

	return status
}

func statusHandler(_ http.ResponseWriter, _ *http.Request) (interface{}, error) {
	return getStatus(), nil
}

func rmslistHandler(_ http.ResponseWriter, req *http.Request) (interface{}, error) {
	forceDownload, _ := strconv.ParseBool(req.FormValue("force-download"))
	band := req.FormValue("band")
	mode := strings.ToLower(req.FormValue("mode"))
	prefix := strings.ToUpper(req.FormValue("prefix"))

	list, err := ReadRMSList(forceDownload, func(r RMS) bool {
		switch {
		case r.URL == nil:
			return false
		case mode != "" && !r.IsMode(mode):
			return false
		case band != "" && !r.IsBand(band):
			return false
		case prefix != "" && !strings.HasPrefix(r.Callsign, prefix):
			return false
		default:
			return true
		}
	})
	if err != nil {
		log.Println(err)
		return nil, err
	}
	sort.Sort(byDist(list))
	return list, nil
}

func qsyHandler(_ http.ResponseWriter, req *http.Request) (interface{}, error) {
	type QSYPayload struct {
		Transport string      `json:"transport"`
		Freq      json.Number `json:"freq"`
	}
	var payload QSYPayload
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		return nil, HTTPError{err, http.StatusBadRequest}
	}

	rig, rigName, ok, err := VFOForTransport(payload.Transport)
	switch {
	case rigName == "":
		// Either unsupported mode or no rig configured for this transport
		err := errors.New("unsupported mode / no rig configured for this transport")
		return nil, HTTPError{err, http.StatusServiceUnavailable}
	case !ok:
		// A rig is configured, but not loaded properly
		err := fmt.Errorf("QSY failed: hamlib rig '%s' not loaded", rigName)
		log.Println(err)
		return nil, err
	case err != nil:
		err = fmt.Errorf("QSY failed: %w", err)
		log.Println(err)
		return nil, err
	default:
		if _, _, err := setFreq(rig, string(payload.Freq)); err != nil {
			err = fmt.Errorf("QSY failed: %w", err)
			log.Println(err)
			return nil, err
		}
		return payload, nil
	}
}

func positionHandler(_ http.ResponseWriter, req *http.Request) (interface{}, error) {
	// Throw error if GPSd http endpoint is not enabled
	if !config.GPSd.EnableHTTP || config.GPSd.Addr == "" {
		return nil, errors.New("GPSd not enabled or address not set in config file")
	}

	host, _, _ := net.SplitHostPort(req.RemoteAddr)
	log.Printf("Location data from GPSd served to %s", host)

	conn, err := gpsd.Dial(config.GPSd.Addr)
	if err != nil {
		// do not pass error message to response as GPSd address might be leaked
		return nil, errors.New("GPSd Dial failed")
	}
	defer conn.Close()

	conn.Watch(true)

	pos, err := conn.NextPosTimeout(5 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("GPSd get next position failed: %w", err)
	}

	if config.GPSd.UseServerTime {
		pos.Time = time.Now()
	}

	return pos, nil
}

func DisconnectHandler(_ http.ResponseWriter, req *http.Request) (interface{}, error) {
	dirty, _ := strconv.ParseBool(req.FormValue("dirty"))
	if ok := abortActiveConnection(dirty); !ok {
		return nil, HTTPError{errors.New("Not available"), http.StatusBadRequest}
	}
	return struct{}{}, nil
}

func ConnectHandler(_ http.ResponseWriter, req *http.Request) (interface{}, error) {
	connectStr := req.FormValue("url")

	nMsgs := mbox.InboxCount()

	if success := Connect(connectStr); !success {
		return nil, errors.New("Session failure")
	}

	return struct {
		NumReceived int
	}{
		mbox.InboxCount() - nMsgs,
	}, nil
}

func mailboxHandler(_ http.ResponseWriter, r *http.Request) (interface{}, error) {
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
		return nil, ErrNotFound
	}

	if err != nil {
		log.Println(err)
		return nil, err
	}

	sort.Sort(sort.Reverse(fbb.ByDate(messages)))

	jsonSlice := make([]JSONMessage, len(messages))
	for i, msg := range messages {
		jsonSlice[i] = JSONMessage{Message: msg}
	}
	return jsonSlice, nil
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

func messageDeleteHandler(_ http.ResponseWriter, r *http.Request) (interface{}, error) {
	box, mid := mux.Vars(r)["box"], mux.Vars(r)["mid"]

	file := filepath.Clean(filepath.Join(mbox.MBoxPath, box, mid+mailbox.Ext))
	if err := isInPath(mbox.MBoxPath, file); err != nil {
		log.Println("Malicious source path in move:", err)
		return nil, HTTPError{err, http.StatusBadRequest}
	}

	err := os.Remove(file)
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}

	return "OK", nil
}

func messageHandler(_ http.ResponseWriter, r *http.Request) (interface{}, error) {
	box, mid := mux.Vars(r)["box"], mux.Vars(r)["mid"]

	msg, err := mailbox.OpenMessage(path.Join(mbox.MBoxPath, box, mid+mailbox.Ext))
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	} else if err != nil {
		log.Println(err)
		return nil, err
	}

	return JSONMessage{msg, true}, nil
}

func attachmentHandler(w http.ResponseWriter, r *http.Request) {
	// Attachments are potentially unsanitized HTML and/or javascript.
	// To avoid XSS, we enable the CSP sandbox directive so that these
	// attachments can't call other parts of the API (deny same origin).
	w.Header().Set("Content-Security-Policy", "sandbox allow-forms allow-modals allow-orientation-lock allow-pointer-lock allow-popups allow-popups-to-escape-sandbox allow-presentation allow-scripts")

	// Allow different sandboxed attachments to refer to each other.
	// This can be useful to provide rich HTML content as attachments,
	// without having to bundle it all up in one big file.
	w.Header().Set("Access-Control-Allow-Origin", "null")

	box, mid, attachment := mux.Vars(r)["box"], mux.Vars(r)["mid"], mux.Vars(r)["attachment"]
	composereply, _ := strconv.ParseBool(r.URL.Query().Get("composereply"))
	renderToHtml, _ := strconv.ParseBool(r.URL.Query().Get("rendertohtml"))

	if composereply || renderToHtml {
		// no-store is needed for displaying and replying to Winlink form-based messages
		w.Header().Set("Cache-Control", "no-store")
	}

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

		if !renderToHtml {
			http.ServeContent(w, r, f.Name(), msg.Date(), bytes.NewReader(f.Data()))
			return
		}

		formRendered, err := formsMgr.RenderForm(f.Data(), composereply)
		if err != nil {
			log.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		http.ServeContent(w, r, f.Name()+".html", msg.Date(), bytes.NewReader([]byte(formRendered)))
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

	_, _ = fmt.Fprint(&out, "<p>")

	scanner := bufio.NewScanner(buf)

	var blockquote int
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			_, _ = fmt.Fprint(&out, "</p><p>")
			continue
		}

		depth := blockquoteDepth(line)
		for depth != blockquote {
			if depth > blockquote {
				_, _ = fmt.Fprintf(&out, "</p><blockquote><p>")
				blockquote++
			} else {
				_, _ = fmt.Fprintf(&out, "</p></blockquote><p>")
				blockquote--
			}
		}
		line = line[depth:]

		line = htmlEncode(line)
		line = linkify(line)

		_, _ = fmt.Fprint(&out, line+"\n")
	}

	for ; blockquote > 0; blockquote-- {
		_, _ = fmt.Fprintf(&out, "</p></blockquote>")
	}

	_, _ = fmt.Fprint(&out, "</p>")
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
	str = strings.ReplaceAll(str, ">", "&gt;")
	str = strings.ReplaceAll(str, "<", "&lt;")
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
