// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"net"
	"net/http"
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
	"github.com/la5nta/pat/internal/signalk"
	"github.com/la5nta/pat/web"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-version"
	"github.com/la5nta/wl2k-go/catalog"
	"github.com/la5nta/wl2k-go/transport/ardop"
	"github.com/n8jja/Pat-Vara/vara"
	"github.com/pd0mz/go-maidenhead"
)

type HTTPError struct {
	error
	StatusCode int
}

func ListenAndServe(ctx context.Context, a *app.App, addr string) error {
	log.Printf("Starting HTTP service (http://%s)...", addr)

	if host, _, _ := net.SplitHostPort(addr); host == "" && a.Config().GPSd.EnableHTTP {
		// TODO: maybe make a popup showing the warning ont the web UI?
		fmt.Fprintf(os.Stderr, "\nWARNING: You have enable GPSd HTTP endpoint (enable_http). You might expose"+
			"\n         your current position to anyone who has access to the Pat web interface!\n\n")
	}

	handler := NewHandler(a)
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

func NewHandler(app *app.App) *Handler {
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
	r.HandleFunc("/api/coords_to_locator", h.coordsToLocatorHandler).Methods("POST")
	r.HandleFunc("/api/qsy", h.qsyHandler).Methods("POST")
	r.HandleFunc("/api/rmslist", h.rmslistHandler).Methods("GET")

	r.HandleFunc("/api/config", h.configHandler).Methods("GET", "PUT")
	r.HandleFunc("/api/config/connect_aliases", h.connectAliasesHandler).Methods("GET")
	r.HandleFunc("/api/config/connect_aliases/{alias}", h.connectAliasHandler).Methods("GET", "PUT", "DELETE")

	r.HandleFunc("/api/reload", h.reloadHandler).Methods("POST")
	r.HandleFunc("/api/bandwidths", h.bandwidthsHandler).Methods("GET")
	r.HandleFunc("/api/connect_aliases", h.connectAliasesHandler).Methods("GET") // DEPRECATED: Use /api/config/connect_aliases.
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

	r.HandleFunc("/ws", h.wsHandler)
	r.PathPrefix("/ui").Handler(web.UIHandler(h.Options().MyCall))
	r.PathPrefix("/dist").Handler(web.DistHandler())
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/ui", http.StatusFound) })

	return h
}

func (h Handler) connectAliasesHandler(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(h.Config().ConnectAliases)
}

func (h Handler) connectAliasHandler(w http.ResponseWriter, r *http.Request) {
	// Make a copy of the map to avoid concurrenct read/write of the "live" map
	currentAliases := maps.Clone(h.Config().ConnectAliases)

	alias := mux.Vars(r)["alias"]
	switch r.Method {
	case http.MethodGet:
		v, ok := currentAliases[alias]
		if !ok {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(v)
	case http.MethodDelete:
		delete(currentAliases, alias)
		if err := h.SetConnectAliases(currentAliases); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		var v string
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		currentAliases[alias] = v
		if err := h.SetConnectAliases(currentAliases); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(v)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
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

	// Sort by predictions if we have more than 1/3 entries with predictions,
	// otherwise sort by distance.
	nPredictions := 0
	for _, rms := range list {
		if rms.Prediction != nil {
			nPredictions++
		}
	}
	if nPredictions > len(list)/3 {
		sort.Sort(sort.Reverse(app.ByLinkQuality(list)))
	} else {
		sort.Sort(app.ByDist(list))
	}

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
	host, _, _ := net.SplitHostPort(req.RemoteAddr)

	// Try Signal K first if enabled
	if h.Config().SignalK.Enable {
		log.Printf("Location data from Signal K served to %s", host)

		conn, err := signalk.DialWithToken(h.Config().SignalK.Addr, h.Config().SignalK.UseServerTime)
		if err == nil {
			defer conn.Close()

			pos, err := conn.NextPosTimeout(5 * time.Second)
			if err == nil {
				_ = json.NewEncoder(w).Encode(gpsd.Position{
					Lat:   pos.Lat,
					Lon:   pos.Lon,
					Alt:   pos.Alt,
					Track: pos.Track,
					Speed: pos.Speed,
					Time:  pos.Time,
				})
				return
			}
		}
	}

	// Fall back to GPSd
	if !h.Config().GPSd.EnableHTTP || h.Config().GPSd.Addr == "" {
		http.Error(w, "No position source enabled (Signal K or GPSd)", http.StatusInternalServerError)
		return
	}

	log.Printf("Location data from GPSd served to %s", host)

	conn, err := gpsd.Dial(h.Config().GPSd.Addr)
	if err != nil {
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

	// Security: Prevent GPSd EnableHTTP from being changed via web interface
	if newConfig.GPSd.EnableHTTP != currentConfig.GPSd.EnableHTTP {
		http.Error(w, "GPSd EnableHTTP setting cannot be changed via web interface for security reasons. Please edit the configuration file manually.", http.StatusForbidden)
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

func (h Handler) coordsToLocatorHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	point := maidenhead.NewPoint(req.Lat, req.Lon)
	locator, err := point.GridSquare()
	if err != nil {
		http.Error(w, "Failed to convert coordinates to locator: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Locator string `json:"locator"`
	}{Locator: locator})
}
