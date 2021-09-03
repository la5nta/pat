// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"context"
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
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/la5nta/wl2k-go/transport/ardop"

	"github.com/la5nta/pat/internal/buildinfo"
	"github.com/la5nta/pat/internal/gpsd"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/la5nta/wl2k-go/catalog"
	"github.com/la5nta/wl2k-go/fbb"
	"github.com/la5nta/wl2k-go/mailbox"
	"github.com/microcosm-cc/bluemonday"
	"github.com/n8jja/Pat-Vara/vara"
)

//go:embed web/dist/**
var embeddedFS embed.FS

var staticContent fs.FS

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

var websocketHub *WSHub

func init() {
	var err error
	staticContent, err = fs.Sub(embeddedFS, "web")
	if err != nil {
		panic(err)
	}
}

func devServerAddr() string { return strings.TrimSuffix(os.Getenv("PAT_WEB_DEV_ADDR"), "/") }

func ListenAndServe(ctx context.Context, addr string) error {
	log.Printf("Starting HTTP service (http://%s)...", addr)

	if host, _, _ := net.SplitHostPort(addr); host == "" && config.GPSd.EnableHTTP {
		// TODO: maybe make a popup showing the warning ont the web UI?
		_, _ = fmt.Fprintf(logWriter, "\nWARNING: You have enable GPSd HTTP endpoint (enable_http). You might expose"+
			"\n         your current position to anyone who has access to the Pat web interface!\n\n")
	}

	r := mux.NewRouter()
	r.HandleFunc("/api/bandwidths", bandwidthsHandler).Methods("GET")
	r.HandleFunc("/api/connect_aliases", connectAliasesHandler).Methods("GET")
	r.HandleFunc("/api/connect", ConnectHandler)
	r.HandleFunc("/api/formcatalog", formsMgr.GetFormsCatalogHandler).Methods("GET")
	r.HandleFunc("/api/form", formsMgr.PostFormDataHandler).Methods("POST")
	r.HandleFunc("/api/form", formsMgr.GetFormDataHandler).Methods("GET")
	r.HandleFunc("/api/forms", formsMgr.GetFormTemplateHandler).Methods("GET")
	r.HandleFunc("/api/formsUpdate", formsMgr.UpdateFormTemplatesHandler).Methods("POST")
	r.HandleFunc("/api/disconnect", DisconnectHandler)
	r.HandleFunc("/api/mailbox/{box}", mailboxHandler).Methods("GET")
	r.HandleFunc("/api/mailbox/{box}/{mid}", messageHandler).Methods("GET")
	r.HandleFunc("/api/mailbox/{box}/{mid}", messageDeleteHandler).Methods("DELETE")
	r.HandleFunc("/api/mailbox/{box}/{mid}/{attachment}", attachmentHandler).Methods("GET")
	r.HandleFunc("/api/mailbox/{box}/{mid}/read", readHandler).Methods("POST")
	r.HandleFunc("/api/mailbox/{box}", postMessageHandler).Methods("POST")
	r.HandleFunc("/api/posreport", postPositionHandler).Methods("POST")
	r.HandleFunc("/api/status", statusHandler).Methods("GET")
	r.HandleFunc("/api/current_gps_position", positionHandler).Methods("GET")
	r.HandleFunc("/api/qsy", qsyHandler).Methods("POST")
	r.HandleFunc("/api/rmslist", rmslistHandler).Methods("GET")
	r.HandleFunc("/ws", wsHandler)
	r.HandleFunc("/ui", uiHandler()).Methods("GET")
	r.HandleFunc("/", rootHandler).Methods("GET")

	http.Handle("/", r)
	http.Handle("/dist/", distHandler())

	websocketHub = NewWSHub()

	srv := http.Server{Addr: addr}
	errs := make(chan error, 1)
	go func() {
		errs <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Println("Shutting down HTTP server...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
		return nil
	case err := <-errs:
		return err
	}
}

func distHandler() http.Handler {
	switch target := devServerAddr(); {
	case target != "":
		targetURL, err := url.Parse(target)
		if err != nil {
			log.Fatalf("invalid proxy target URL: %v", err)
		}
		return httputil.NewSingleHostReverseProxy(targetURL)
	default:
		return http.FileServer(http.FS(staticContent))
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/ui", http.StatusFound)
}

func connectAliasesHandler(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(config.ConnectAliases)
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
	_ = r.Body.Close()

	if pos.Date.IsZero() {
		pos.Date = time.Now()
	}

	// Post to outbox
	msg := pos.Message(fOptions.MyCall)
	if err := mbox.AddOut(msg); err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		_, _ = fmt.Fprintln(w, "Position update posted")
	}
}

func isInPath(base string, path string) error {
	_, err := filepath.Rel(base, path)
	return err
}

func postMessageHandler(w http.ResponseWriter, r *http.Request) {
	box := mux.Vars(r)["box"]
	if box == "out" {
		postOutboundMessageHandler(w, r)
		return
	}

	srcPath := r.Header.Get("X-Pat-SourcePath")
	if srcPath == "" {
		http.Error(w, "Not implemented", http.StatusNotImplemented)
		return
	}

	srcPath, _ = url.PathUnescape(strings.TrimPrefix(srcPath, "/api/mailbox/"))
	srcPath = filepath.Join(mbox.MBoxPath, srcPath+mailbox.Ext)

	// Check that we don't escape our mailbox path
	srcPath = filepath.Clean(srcPath)
	if err := isInPath(mbox.MBoxPath, srcPath); err != nil {
		log.Println("Malicious source path in move:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	targetPath := filepath.Join(mbox.MBoxPath, box, filepath.Base(srcPath))

	if err := os.Rename(srcPath, targetPath); err != nil {
		log.Println("Could not move message:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
	} else {
		_ = json.NewEncoder(w).Encode("OK")
	}
}

func postOutboundMessageHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 * (1024 ^ 2)) // 10Mb
	if err != nil {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
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

	if err := msg.Validate(); err != nil {
		http.Error(w, "Validation error: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Post to outbox
	if err := mbox.AddOut(msg); err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	var buf bytes.Buffer
	_ = msg.Write(&buf)
	_, _ = fmt.Fprintf(w, "Message posted (%.2f kB)", float64(buf.Len()/1024))
}

func attachFile(f *multipart.FileHeader, msg *fbb.Message) error {
	// For some unknown reason, we receive this empty unnamed file when no
	// attachment is provided. Prior to Go 1.10, this was filtered by
	// multipart.Reader.
	if f.Size == 0 && f.Filename == "" {
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

func uiHandler() http.HandlerFunc {
	const indexPath = "dist/index.html"
	templateFunc := func() ([]byte, error) { return fs.ReadFile(staticContent, indexPath) }
	if target := devServerAddr(); target != "" {
		templateFunc = func() ([]byte, error) {
			resp, err := http.Get(target + "/" + indexPath)
			if err != nil {
				return nil, fmt.Errorf("dev server not reachable: %w", err)
			}
			defer resp.Body.Close()
			return io.ReadAll(resp.Body)
		}
	}

	return func(w http.ResponseWriter, _ *http.Request) {
		data, err := templateFunc()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		t, err := template.New("index.html").Parse(string(data))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmplData := struct{ AppName, Version, Mycall string }{buildinfo.AppName, buildinfo.VersionString(), fOptions.MyCall}
		if err := t.Execute(w, tmplData); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
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

func statusHandler(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(getStatus())
}

func bandwidthsHandler(w http.ResponseWriter, req *http.Request) {
	type BandwidthResponse struct {
		Mode       string   `json:"mode"`
		Bandwidths []string `json:"bandwidths"`
		Default    string   `json:"default,omitempty"`
	}
	mode := strings.ToLower(req.FormValue("mode"))
	resp := BandwidthResponse{Mode: mode, Bandwidths: []string{}}
	switch mode {
	case MethodPactor:
		fallthrough
	case MethodAX25:
		fallthrough
	case MethodTelnet:
		fallthrough
	case MethodVaraFM:
		fallthrough
	case MethodSerialTNC:
		// bandwidth not supported, return empty list
	case MethodArdop:
		for _, bw := range ardop.Bandwidths() {
			resp.Bandwidths = append(resp.Bandwidths, bw.String())
		}
		if bw := config.Ardop.ARQBandwidth; !bw.IsZero() {
			resp.Default = bw.String()
		}
	case MethodVaraHF:
		resp.Bandwidths = vara.Bandwidths()
		if bw := config.VaraHF.Bandwidth; bw != 0 {
			resp.Default = fmt.Sprintf("%d", bw)
		}
	default:
		http.Error(w, "mode not recognized", http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func rmslistHandler(w http.ResponseWriter, req *http.Request) {
	forceDownload, _ := strconv.ParseBool(req.FormValue("force-download"))
	band := req.FormValue("band")
	mode := strings.ToLower(req.FormValue("mode"))
	prefix := strings.ToUpper(req.FormValue("prefix"))

	list, err := ReadRMSList(req.Context(), forceDownload, func(r RMS) bool {
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sort.Sort(byDist(list))
	err = json.NewEncoder(w).Encode(list)
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func qsyHandler(w http.ResponseWriter, req *http.Request) {
	type QSYPayload struct {
		Transport string      `json:"transport"`
		Freq      json.Number `json:"freq"`
	}
	var payload QSYPayload
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rig, rigName, ok, err := VFOForTransport(payload.Transport)
	switch {
	case rigName == "":
		// Either unsupported mode or no rig configured for this transport
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	case !ok:
		// A rig is configured, but not loaded properly
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("QSY failed: Hamlib rig '%s' not loaded.", rigName)
	case err != nil:
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("QSY failed: %v", err)
	default:
		if _, _, err := setFreq(rig, string(payload.Freq)); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("QSY failed: %v", err)
			return
		}
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func positionHandler(w http.ResponseWriter, req *http.Request) {
	// Throw error if GPSd http endpoint is not enabled
	if !config.GPSd.EnableHTTP || config.GPSd.Addr == "" {
		http.Error(w, "GPSd not enabled or address not set in config file", http.StatusInternalServerError)
		return
	}

	host, _, _ := net.SplitHostPort(req.RemoteAddr)
	log.Printf("Location data from GPSd served to %s", host)

	conn, err := gpsd.Dial(config.GPSd.Addr)
	if err != nil {
		// do not pass error message to response as GPSd address might be leaked
		http.Error(w, "GPSd Dial failed", http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	conn.Watch(true)

	pos, err := conn.NextPosTimeout(5 * time.Second)
	if err != nil {
		http.Error(w, "GPSd get next position failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if config.GPSd.UseServerTime {
		pos.Time = time.Now()
	}

	_ = json.NewEncoder(w).Encode(pos)
}

func DisconnectHandler(w http.ResponseWriter, req *http.Request) {
	dirty, _ := strconv.ParseBool(req.FormValue("dirty"))
	if ok := abortActiveConnection(dirty); !ok {
		w.WriteHeader(http.StatusBadRequest)
	}
	_ = json.NewEncoder(w).Encode(struct{}{})
}

func ConnectHandler(w http.ResponseWriter, req *http.Request) {
	connectStr := req.FormValue("url")

	nMsgs := mbox.InboxCount()

	if success := Connect(connectStr); !success {
		http.Error(w, "Session failure", http.StatusInternalServerError)
	}

	_ = json.NewEncoder(w).Encode(struct {
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
	_ = json.NewEncoder(w).Encode(jsonSlice)
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

func messageDeleteHandler(w http.ResponseWriter, r *http.Request) {
	box, mid := mux.Vars(r)["box"], mux.Vars(r)["mid"]

	file := filepath.Clean(filepath.Join(mbox.MBoxPath, box, mid+mailbox.Ext))
	if err := isInPath(mbox.MBoxPath, file); err != nil {
		log.Println("Malicious source path in move:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := os.Remove(file)
	if os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	_ = json.NewEncoder(w).Encode("OK")
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

	_ = json.NewEncoder(w).Encode(JSONMessage{msg, true})
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
