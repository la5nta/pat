// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package api

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/la5nta/pat/app"
	"github.com/la5nta/pat/cfg"
	"github.com/la5nta/pat/internal/buildinfo"
	"github.com/la5nta/pat/internal/gpsd"
	"github.com/la5nta/pat/internal/patapi"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-version"
	"github.com/la5nta/wl2k-go/catalog"
	"github.com/la5nta/wl2k-go/transport/ardop"
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

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.r.ServeHTTP(w, r) }

func NewHandler(app *app.App, staticContent fs.FS) *Handler {
	r := mux.NewRouter()
	h := &Handler{app, NewWSHub(app), r}

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
	r.HandleFunc("/api/reload", h.reloadHandler).Methods("POST")
	r.HandleFunc("/api/bandwidths", h.bandwidthsHandler).Methods("GET")
	r.HandleFunc("/api/connect_aliases", h.connectAliasesHandler).Methods("GET")
	r.HandleFunc("/api/new-release-check", h.newReleaseCheckHandler).Methods("GET")

	r.HandleFunc("/api/formcatalog", h.FormsManager().GetFormsCatalogHandler).Methods("GET")
	r.HandleFunc("/api/form", h.FormsManager().PostFormDataHandler(h.Mailbox().MBoxPath)).Methods("POST")
	r.HandleFunc("/api/template", h.FormsManager().GetTemplateDataHandler(h.Mailbox().MBoxPath)).Methods("GET")
	r.HandleFunc("/api/form", h.FormsManager().GetFormDataHandler).Methods("GET")
	r.HandleFunc("/api/forms", h.FormsManager().GetFormTemplateHandler).Methods("GET")
	r.PathPrefix("/api/forms/").Handler(http.StripPrefix("/api/forms/", http.HandlerFunc(h.FormsManager().GetFormAssetHandler))).Methods("GET")
	r.HandleFunc("/api/formsUpdate", h.FormsManager().UpdateFormTemplatesHandler).Methods("POST")

	r.HandleFunc("/api/winlink-account/password-recovery-email", h.winlinkPasswordRecoveryEmailHandler).Methods("GET", "PUT")
	r.HandleFunc("/api/winlink-account/registration", h.winlinkAccountRegistrationHandler).Methods("GET", "POST")

	r.PathPrefix("/dist/").Handler(h.distHandler(staticContent))
	r.HandleFunc("/ws", h.wsHandler)
	r.HandleFunc("/ui", h.uiHandler(staticContent, "dist/index.html")).Methods("GET")
	r.HandleFunc("/ui/config", h.uiHandler(staticContent, "dist/config.html")).Methods("GET")
	r.HandleFunc("/ui/template", h.uiHandler(staticContent, "dist/template.html")).Methods("GET")
	r.HandleFunc("/", h.rootHandler).Methods("GET")

	return h
}

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

func (h Handler) postPositionHandler(w http.ResponseWriter, r *http.Request) {
	var pos catalog.PosReport
	if err := json.NewDecoder(r.Body).Decode(&pos); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if pos.Date.IsZero() {
		pos.Date = time.Now()
	}
	msg := pos.Message(h.Options().MyCall)

	// Post to outbox
	if err := h.Mailbox().AddOut(msg); err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintln(w, "Position update posted")
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
	var (
		forceDownload, _ = strconv.ParseBool(req.FormValue("force-download"))
		band             = req.FormValue("band")
		mode             = strings.ToLower(req.FormValue("mode"))
		prefix           = strings.ToUpper(req.FormValue("prefix"))
	)
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

	sort.Sort(sort.Reverse(app.ByLinkQuality(list)))
	json.NewEncoder(w).Encode(list)
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

func (h Handler) reloadHandler(w http.ResponseWriter, r *http.Request) {
	if err := h.App.Reload(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
