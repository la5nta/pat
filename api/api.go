// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package api

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

	"github.com/la5nta/pat/app"
	"github.com/la5nta/pat/cfg"
	"github.com/la5nta/pat/internal/buildinfo"
	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/pat/internal/directories"
	"github.com/la5nta/pat/internal/gpsd"
	"github.com/la5nta/pat/internal/patapi"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-version"
	"github.com/la5nta/wl2k-go/catalog"
	"github.com/la5nta/wl2k-go/fbb"
	"github.com/la5nta/wl2k-go/mailbox"
	"github.com/la5nta/wl2k-go/transport/ardop"
	"github.com/microcosm-cc/bluemonday"
	"github.com/n8jja/Pat-Vara/vara"
)

// The web/ go:embed directive must be in package main because we can't
// reference ../ here. main assigns this variable on init.
var EmbeddedFS embed.FS

type HTTPError struct {
	error
	StatusCode int
}

func devServerAddr() string { return strings.TrimSuffix(os.Getenv("PAT_WEB_DEV_ADDR"), "/") }

func ListenAndServe(ctx context.Context, a *app.App, addr string) error {
	log.Printf("Starting HTTP service (http://%s)...", addr)

	if host, _, _ := net.SplitHostPort(addr); host == "" && a.Config().GPSd.EnableHTTP {
		// TODO: maybe make a popup showing the warning ont the web UI?
		fmt.Fprintf(os.Stderr, "\nWARNING: You have enable GPSd HTTP endpoint (enable_http). You might expose"+
			"\n         your current position to anyone who has access to the Pat web interface!\n\n")
	}

	staticContent, err := fs.Sub(EmbeddedFS, "web")
	if err != nil {
		return err
	}

	handler := NewHandler(a, staticContent)
	go handler.wsHub.WatchMBox(ctx, a.Mailbox())
	if err := a.EnableWebSocket(ctx, handler.wsHub); err != nil {
		return err
	}

	srv := http.Server{
		Addr:    addr,
		Handler: handler,
	}
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

type Handler struct {
	*app.App
	wsHub *WSHub
	r     *mux.Router
}

func NewHandler(app *app.App, staticContent fs.FS) *Handler {
	r := mux.NewRouter()
	h := &Handler{app, NewWSHub(app), r}

	r.HandleFunc("/api/bandwidths", h.bandwidthsHandler).Methods("GET")
	r.HandleFunc("/api/connect_aliases", h.connectAliasesHandler).Methods("GET")
	r.HandleFunc("/api/connect", h.ConnectHandler)
	r.HandleFunc("/api/disconnect", h.DisconnectHandler)
	r.HandleFunc("/api/mailbox/{box}", h.mailboxHandler).Methods("GET")
	r.HandleFunc("/api/mailbox/{box}/{mid}", h.messageHandler).Methods("GET")
	r.HandleFunc("/api/mailbox/{box}/{mid}", h.messageDeleteHandler).Methods("DELETE")
	r.HandleFunc("/api/mailbox/{box}/{mid}/{attachment}", h.attachmentHandler).Methods("GET")
	r.HandleFunc("/api/mailbox/{box}/{mid}/read", h.readHandler).Methods("POST")
	r.HandleFunc("/api/mailbox/{box}", h.postMessageHandler).Methods("POST")
	r.HandleFunc("/api/posreport", h.postPositionHandler).Methods("POST")
	r.HandleFunc("/api/status", h.statusHandler).Methods("GET")
	r.HandleFunc("/api/current_gps_position", h.positionHandler).Methods("GET")
	r.HandleFunc("/api/qsy", h.qsyHandler).Methods("POST")
	r.HandleFunc("/api/rmslist", h.rmslistHandler).Methods("GET")
	r.HandleFunc("/api/config", h.configHandler).Methods("GET", "PUT")
	r.HandleFunc("/api/new-release-check", h.newReleaseCheckHandler).Methods("GET")

	r.HandleFunc("/api/formcatalog", h.FormsManager().GetFormsCatalogHandler).Methods("GET")
	r.HandleFunc("/api/form", h.FormsManager().PostFormDataHandler(h.Mailbox().MBoxPath)).Methods("POST")
	r.HandleFunc("/api/template", h.FormsManager().GetTemplateDataHandler(h.Mailbox().MBoxPath)).Methods("GET")
	r.HandleFunc("/api/form", h.FormsManager().GetFormDataHandler).Methods("GET")
	r.HandleFunc("/api/forms", h.FormsManager().GetFormTemplateHandler).Methods("GET")
	r.PathPrefix("/api/forms/").Handler(http.StripPrefix("/api/forms/", http.HandlerFunc(h.FormsManager().GetFormAssetHandler))).Methods("GET")
	r.HandleFunc("/api/formsUpdate", h.FormsManager().UpdateFormTemplatesHandler).Methods("POST")

	r.PathPrefix("/dist/").Handler(h.distHandler(staticContent))
	r.HandleFunc("/ws", h.wsHandler)
	r.HandleFunc("/ui", h.uiHandler(staticContent, "dist/index.html")).Methods("GET")
	r.HandleFunc("/ui/config", h.uiHandler(staticContent, "dist/config.html")).Methods("GET")
	r.HandleFunc("/ui/template", h.uiHandler(staticContent, "dist/template.html")).Methods("GET")
	r.HandleFunc("/", h.rootHandler).Methods("GET")

	return h
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.r.ServeHTTP(w, r) }

func (h Handler) distHandler(staticContent fs.FS) http.Handler {
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

func (h Handler) rootHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/ui", http.StatusFound)
}

func (h Handler) connectAliasesHandler(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(h.Config().ConnectAliases)
}

func (h Handler) readHandler(w http.ResponseWriter, r *http.Request) {
	var data struct{ Read bool }
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Printf("%s %s: %s", r.Method, r.URL.Path, err)
		return
	}

	box, mid := mux.Vars(r)["box"], mux.Vars(r)["mid"]

	msg, err := mailbox.OpenMessage(path.Join(h.Mailbox().MBoxPath, box, mid+mailbox.Ext))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := mailbox.SetUnread(msg, !data.Read); err != nil {
		log.Printf("%s %s: %s", r.Method, r.URL.Path, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h Handler) postPositionHandler(w http.ResponseWriter, r *http.Request) {
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
	msg := pos.Message(h.Options().MyCall)
	if err := h.Mailbox().AddOut(msg); err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		_, _ = fmt.Fprintln(w, "Position update posted")
	}
}

func (h Handler) postMessageHandler(w http.ResponseWriter, r *http.Request) {
	box := mux.Vars(r)["box"]
	if box == "out" {
		h.postOutboundMessageHandler(w, r)
		return
	}

	srcPath := r.Header.Get("X-Pat-SourcePath")
	if srcPath == "" {
		http.Error(w, "Not implemented", http.StatusNotImplemented)
		return
	}

	srcPath, _ = url.PathUnescape(strings.TrimPrefix(srcPath, "/api/mailbox/"))
	srcPath = filepath.Join(h.Mailbox().MBoxPath, srcPath+mailbox.Ext)

	// Check that we don't escape our mailbox path
	srcPath = filepath.Clean(srcPath)
	if !directories.IsInPath(h.Mailbox().MBoxPath, srcPath) {
		log.Println("Malicious source path in move:", srcPath)
		http.Error(w, "malicious source path", http.StatusBadRequest)
		return
	}

	targetPath := filepath.Join(h.Mailbox().MBoxPath, box, filepath.Base(srcPath))

	if err := os.Rename(srcPath, targetPath); err != nil {
		log.Println("Could not move message:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
	} else {
		_ = json.NewEncoder(w).Encode("OK")
	}
}

func (h Handler) postOutboundMessageHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 * (1024 ^ 2)) // 10Mb
	if err != nil {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	msg := fbb.NewMessage(fbb.Private, h.Options().MyCall)

	// files
	if r.MultipartForm != nil {
		files := r.MultipartForm.File["files"]
		for _, f := range files {
			err := addAttachmentFromMultipartFile(msg, f)
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

	if cookie, err := r.Cookie("forminstance"); err == nil {
		// We must add the attachment files here because it is impossible
		// for the frontend to dynamically add form files due to legacy
		// security vulnerabilities in older HTML specs.
		// The rest of the form data (to, subject, body etc) is added by
		// the frontend.
		formData, ok := h.FormsManager().GetPostedFormData(cookie.Value)
		if !ok {
			debug.Printf("form instance key (%q) not valid", cookie.Value)
			http.Error(w, "form instance key not valid", http.StatusBadRequest)
			return
		}
		for _, f := range formData.Attachments {
			msg.AddFile(f)
		}
	}

	// Other fields
	if v := r.Form["to"]; len(v) == 1 {
		addrs := strings.FieldsFunc(v[0], app.SplitFunc)
		msg.AddTo(addrs...)
	}
	if v := r.Form["cc"]; len(v) == 1 {
		addrs := strings.FieldsFunc(v[0], app.SplitFunc)
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
	if err := h.Mailbox().AddOut(msg); err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	var buf bytes.Buffer
	_ = msg.Write(&buf)
	_, _ = fmt.Fprintf(w, "Message posted (%.2f kB)", float64(buf.Len()/1024))
}

func addAttachmentFromMultipartFile(msg *fbb.Message, f *multipart.FileHeader) error {
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
	defer file.Close()
	if err := app.AddAttachment(msg, f.Filename, f.Header.Get("Content-Type"), file); err != nil {
		return HTTPError{err, http.StatusInternalServerError}
	}
	return nil
}

func (h Handler) wsHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	_ = conn.WriteJSON(struct{ MyCall string }{h.Options().MyCall})
	h.wsHub.Handle(conn)
}

func (h Handler) uiHandler(staticContent fs.FS, templatePath string) http.HandlerFunc {
	templateFunc := func() ([]byte, error) { return fs.ReadFile(staticContent, templatePath) }
	if target := devServerAddr(); target != "" {
		templateFunc = func() ([]byte, error) {
			resp, err := http.Get(target + "/" + templatePath)
			if err != nil {
				return nil, fmt.Errorf("dev server not reachable: %w", err)
			}
			defer resp.Body.Close()
			return io.ReadAll(resp.Body)
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// Redirect to config if no callsign is set and we're not already on config page
		if h.Options().MyCall == "" && r.URL.Path != "/ui/config" {
			http.Redirect(w, r, "/ui/config", http.StatusFound)
			return
		}
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
		tmplData := struct{ AppName, Version, Mycall string }{buildinfo.AppName, buildinfo.VersionString(), h.Options().MyCall}
		if err := t.Execute(w, tmplData); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func (h Handler) statusHandler(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(h.GetStatus())
}

func (h Handler) bandwidthsHandler(w http.ResponseWriter, req *http.Request) {
	type BandwidthResponse struct {
		Mode       string   `json:"mode"`
		Bandwidths []string `json:"bandwidths"`
		Default    string   `json:"default,omitempty"`
	}
	mode := strings.ToLower(req.FormValue("mode"))
	resp := BandwidthResponse{Mode: mode, Bandwidths: []string{}}
	switch mode {
	case app.MethodArdop:
		for _, bw := range ardop.Bandwidths() {
			resp.Bandwidths = append(resp.Bandwidths, bw.String())
		}
		if bw := h.Config().Ardop.ARQBandwidth; !bw.IsZero() {
			resp.Default = bw.String()
		}
	case app.MethodVaraHF:
		resp.Bandwidths = vara.Bandwidths()
		if bw := h.Config().VaraHF.Bandwidth; bw != 0 {
			resp.Default = fmt.Sprintf("%d", bw)
		}
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h Handler) rmslistHandler(w http.ResponseWriter, req *http.Request) {
	forceDownload, _ := strconv.ParseBool(req.FormValue("force-download"))
	band := req.FormValue("band")
	mode := strings.ToLower(req.FormValue("mode"))
	prefix := strings.ToUpper(req.FormValue("prefix"))

	list, err := h.ReadRMSList(req.Context(), forceDownload, func(r app.RMS) bool {
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
	sort.Sort(app.ByDist(list))
	err = json.NewEncoder(w).Encode(list)
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h Handler) qsyHandler(w http.ResponseWriter, req *http.Request) {
	type QSYPayload struct {
		Transport string      `json:"transport"`
		Freq      json.Number `json:"freq"`
	}
	var payload QSYPayload
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rig, rigName, ok, err := h.VFOForTransport(payload.Transport)
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
		if _, _, err := app.SetFreq(rig, string(payload.Freq)); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("QSY failed: %v", err)
			return
		}
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func (h Handler) positionHandler(w http.ResponseWriter, req *http.Request) {
	// Throw error if GPSd http endpoint is not enabled
	if !h.Config().GPSd.EnableHTTP || h.Config().GPSd.Addr == "" {
		http.Error(w, "GPSd not enabled or address not set in config file", http.StatusInternalServerError)
		return
	}

	host, _, _ := net.SplitHostPort(req.RemoteAddr)
	log.Printf("Location data from GPSd served to %s", host)

	conn, err := gpsd.Dial(h.Config().GPSd.Addr)
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

	if h.Config().GPSd.UseServerTime {
		pos.Time = time.Now()
	}

	_ = json.NewEncoder(w).Encode(pos)
}

func (h Handler) DisconnectHandler(w http.ResponseWriter, req *http.Request) {
	dirty, _ := strconv.ParseBool(req.FormValue("dirty"))
	if ok := h.AbortActiveConnection(dirty); !ok {
		w.WriteHeader(http.StatusBadRequest)
	}
	_ = json.NewEncoder(w).Encode(struct{}{})
}

func (h Handler) ConnectHandler(w http.ResponseWriter, req *http.Request) {
	connectStr := req.FormValue("url")

	nMsgs := h.Mailbox().InboxCount()

	if success := h.Connect(connectStr); !success {
		http.Error(w, "Session failure", http.StatusInternalServerError)
	}

	_ = json.NewEncoder(w).Encode(struct{ NumReceived int }{
		h.Mailbox().InboxCount() - nMsgs,
	})
}

func (h Handler) mailboxHandler(w http.ResponseWriter, r *http.Request) {
	box := mux.Vars(r)["box"]

	var messages []*fbb.Message
	var err error

	switch box {
	case "in":
		messages, err = h.Mailbox().Inbox()
	case "out":
		messages, err = h.Mailbox().Outbox()
	case "sent":
		messages, err = h.Mailbox().Sent()
	case "archive":
		messages, err = h.Mailbox().Archive()
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

func (h Handler) messageDeleteHandler(w http.ResponseWriter, r *http.Request) {
	box, mid := mux.Vars(r)["box"], mux.Vars(r)["mid"]

	file := filepath.Clean(filepath.Join(h.Mailbox().MBoxPath, box, mid+mailbox.Ext))
	if !directories.IsInPath(h.Mailbox().MBoxPath, file) {
		log.Println("Malicious source path in move:", file)
		http.Error(w, "malicious source path", http.StatusBadRequest)
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

func (h Handler) messageHandler(w http.ResponseWriter, r *http.Request) {
	box, mid := mux.Vars(r)["box"], mux.Vars(r)["mid"]

	msg, err := mailbox.OpenMessage(path.Join(h.Mailbox().MBoxPath, box, mid+mailbox.Ext))
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

func (h Handler) attachmentHandler(w http.ResponseWriter, r *http.Request) {
	// Attachments are potentially unsanitized HTML and/or javascript.
	// To avoid XSS, we enable the CSP sandbox directive so that these
	// attachments can't call other parts of the API (deny same origin).
	w.Header().Set("Content-Security-Policy", "sandbox allow-forms allow-modals allow-orientation-lock allow-pointer-lock allow-popups allow-popups-to-escape-sandbox allow-presentation allow-scripts")

	// Allow different sandboxed attachments to refer to each other.
	// This can be useful to provide rich HTML content as attachments,
	// without having to bundle it all up in one big file.
	w.Header().Set("Access-Control-Allow-Origin", "null")

	box, mid, attachment := mux.Vars(r)["box"], mux.Vars(r)["mid"], mux.Vars(r)["attachment"]
	inReplyTo := r.URL.Query().Get("in-reply-to")
	renderToHtml, _ := strconv.ParseBool(r.URL.Query().Get("rendertohtml"))

	if inReplyTo != "" || renderToHtml {
		// no-store is needed for displaying and replying to Winlink form-based messages
		w.Header().Set("Cache-Control", "no-store")
	}

	msg, err := mailbox.OpenMessage(path.Join(h.Mailbox().MBoxPath, box, mid+mailbox.Ext))
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

		var inReplyToMsg *fbb.Message
		if inReplyTo != "" {
			var err error
			inReplyToMsg, err = mailbox.OpenMessage(path.Join(h.Mailbox().MBoxPath, inReplyTo+mailbox.Ext))
			if err != nil {
				err = fmt.Errorf("Failed to load in-reply-to message (%q): %v", inReplyTo, err)
				log.Println(err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		formRendered, err := h.FormsManager().RenderForm(f.Data(), inReplyToMsg, inReplyTo)
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

func (h Handler) newReleaseCheckHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	release, err := patapi.GetLatestVersion(ctx)
	if err != nil {
		http.Error(w, "Error getting latest version: "+err.Error(), http.StatusInternalServerError)
		return
	}

	currentVer, err := version.NewVersion(buildinfo.Version)
	if err != nil {
		http.Error(w, "Invalid current version format: "+err.Error(), http.StatusInternalServerError)
		return
	}
	latestVer, err := version.NewVersion(release.Version)
	if err != nil {
		http.Error(w, "Invalid latest version format: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if currentVer.Compare(latestVer) >= 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(release)
}

func (h Handler) configHandler(w http.ResponseWriter, r *http.Request) {
	const RedactedPassword = "[REDACTED]"

	currentConfig, err := app.LoadConfig(h.Options().ConfigPath, cfg.DefaultConfig)
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if r.Method == "GET" {
		if currentConfig.SecureLoginPassword != "" {
			// Redact password before sending over unsafe channel.
			currentConfig.SecureLoginPassword = RedactedPassword
		}
		json.NewEncoder(w).Encode(currentConfig)
		return
	}

	var newConfig cfg.Config
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Reset redacted password if it was unmodified (to retain old value)
	if newConfig.SecureLoginPassword == RedactedPassword {
		newConfig.SecureLoginPassword = currentConfig.SecureLoginPassword
	}

	if err := app.WriteConfig(newConfig, h.Options().ConfigPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	_ = json.NewEncoder(w).Encode("OK")
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
