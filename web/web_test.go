package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDistHandler(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{
			name:       "valid asset",
			path:       "/dist/js/app.js",
			wantStatus: http.StatusOK,
		},
		{
			name:       "non-existent asset",
			path:       "/dist/foobar",
			wantStatus: http.StatusNotFound,
		},
	}
	handler := DistHandler()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestUIHandler(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{
			name:       "main UI",
			path:       "/ui",
			wantStatus: http.StatusOK,
		},
		{
			name:       "config UI",
			path:       "/ui/config",
			wantStatus: http.StatusOK,
		},
		{
			name:       "non-existent UI page",
			path:       "/ui/foobar",
			wantStatus: http.StatusNotFound,
		},
	}
	handler := UIHandler("N0CALL")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestRenderTemplateEditor(t *testing.T) {
	const templateText = "Hello, world!"
	rec := httptest.NewRecorder()
	if err := RenderTemplateEditor(rec, templateText); err != nil {
		t.Fatalf("RenderTemplateEditor returned error: %v", err)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("got Content-Type %q, want text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), templateText) {
		t.Error("rendered page does not contain the template text")
	}
}
