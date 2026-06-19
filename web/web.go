// Package web provides HTTP handlers for serving the web UI.
package web

import (
	"embed"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/la5nta/pat/internal/buildinfo"

	"github.com/gorilla/mux"
)

//go:embed dist/**
var embeddedFS embed.FS

func devServerAddr() string { return strings.TrimSuffix(os.Getenv("PAT_WEB_DEV_ADDR"), "/") }

// DistHandler returns an HTTP handler that serves the static files for the web UI.
func DistHandler() http.Handler {
	switch target := devServerAddr(); {
	case target != "":
		targetURL, err := url.Parse(target)
		if err != nil {
			log.Fatalf("invalid proxy target URL: %v", err)
		}
		return httputil.NewSingleHostReverseProxy(targetURL)
	default:
		return http.FileServer(http.FS(embeddedFS))
	}
}

// UIHandler returns an HTTP handler that serves the UI pages with the given callsign.
func UIHandler(mycall string) http.Handler {
	r := mux.NewRouter()
	r.HandleFunc("/ui", templateHandler("dist/index.html", mycall)).Methods("GET")
	r.HandleFunc("/ui/config", templateHandler("dist/config.html", mycall)).Methods("GET")
	return r
}

// RenderTemplateEditor renders the text-only template editor page for the given template text.
func RenderTemplateEditor(w http.ResponseWriter, templateText string) error {
	type templateEditorData struct{ TemplateText string }
	t, err := loadTemplate("dist/template.html")
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return t.Execute(w, templateEditorData{TemplateText: templateText})
}

func templateHandler(templatePath string, mycall string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Redirect to config if no callsign is set and we're not already on config page
		if mycall == "" && r.URL.Path != "/ui/config" {
			http.Redirect(w, r, "/ui/config", http.StatusFound)
			return
		}
		t, err := loadTemplate(templatePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmplData := struct{ AppName, Version, Mycall string }{buildinfo.AppName, buildinfo.VersionString(), mycall}
		if err := t.Execute(w, tmplData); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func loadTemplate(templatePath string) (*template.Template, error) {
	if devServer := devServerAddr(); devServer != "" {
		// Dev mode: fetch from dev server
		resp, err := http.Get(devServer + "/" + templatePath)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return template.New(path.Base(templatePath)).Parse(string(data))
	}

	// Load from embedded FS
	return template.ParseFS(embeddedFS, templatePath)
}
