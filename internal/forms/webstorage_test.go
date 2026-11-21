package forms

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInjectHeadScript(t *testing.T) {
	script := `<script>SHIM</script>`
	tests := []struct {
		name   string
		doc    string
		expect string
	}{
		{
			name:   "after head",
			doc:    `<html><head><title>t</title></head><body></body></html>`,
			expect: `<html><head>` + script + `<title>t</title></head><body></body></html>`,
		},
		{
			name:   "uppercase head with attributes",
			doc:    `<HTML><HEAD lang="en"><script>original</script></HEAD></HTML>`,
			expect: `<HTML><HEAD lang="en">` + script + `<script>original</script></HEAD></HTML>`,
		},
		{
			name:   "doctype preserved",
			doc:    "<!DOCTYPE html>\n<html><head></head><body><script>x</script></body></html>",
			expect: "<!DOCTYPE html>\n<html><head>" + script + "</head><body><script>x</script></body></html>",
		},
		{
			name:   "no head, after html",
			doc:    `<html><body><script>x</script></body></html>`,
			expect: `<html>` + script + `<body><script>x</script></body></html>`,
		},
		{
			name:   "header element is not a head tag",
			doc:    `<html><body><header>x</header></body></html>`,
			expect: `<html>` + script + `<body><header>x</header></body></html>`,
		},
		{
			name:   "no head or html, prepended",
			doc:    `<p>fragment</p>`,
			expect: script + `<p>fragment</p>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := injectHeadScript(tt.doc, script); got != tt.expect {
				t.Errorf("got %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestFillFormTemplateInjectsWebStorageShim(t *testing.T) {
	dir := t.TempDir()
	formPath := filepath.Join(dir, "form.html")
	body := `<html><head><title>{MsgSender}</title></head><body></body></html>`
	if err := os.WriteFile(formPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	m := &Manager{config: Config{FormsPath: dir, MyCall: "LA5NTA"}}
	out, err := m.fillFormTemplate(formPath, nil, "/api/form", nil)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, webStorageShim) {
		t.Error("Web Storage shim not injected into served form")
	}
	// The shim must be the first script executed by the document.
	if idx := strings.Index(out, webStorageShim); idx > strings.Index(out, "<title>") {
		t.Error("Web Storage shim not injected right after <head>")
	}
	// Injection must not interfere with the placeholder substitutions.
	if !strings.Contains(out, "<title>LA5NTA</title>") {
		t.Error("insertion tag substitution missing from served form")
	}
}
