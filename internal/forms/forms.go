// Copyright 2020 Rainer Grosskopf (KI7RMJ). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// Processes Winlink-compatible message template (aka Winlink forms)

package forms

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/la5nta/pat/cfg"
	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/pat/internal/directories"
	"github.com/la5nta/pat/internal/gpsd"
)

const formsVersionInfoURL = "https://api.getpat.io/v1/forms/standard-templates/latest"

const (
	htmlFileExt  = ".html"
	txtFileExt   = ".txt"
	replyFileExt = ".0"
)

// Manager manages the forms subsystem
type Manager struct {
	config Config

	// postedFormData serves as an kv-store holding intermediate data for
	// communicating form values submitted by the served HTML form files to
	// the rest of the app.
	//
	// When the web frontend POSTs the form template data, this map holds
	// the POST'ed data. Each form composer instance renders into another
	// browser tab, and has a unique instance cookie. This instance cookie
	// is the key into the map, so that we can keep the values from
	// different form authoring sessions separate from each other.
	postedFormData struct {
		mu sync.RWMutex
		m  map[string]Message
	}
}

// Config passes config options to the forms package
type Config struct {
	FormsPath  string
	MyCall     string
	Locator    string
	AppVersion string
	LineReader func() string
	UserAgent  string
	GPSd       cfg.GPSdConfig
}

// FormFolder is a folder with forms. A tree structure with Form leaves and sub-Folder branches
type FormFolder struct {
	Name      string       `json:"name"`
	Path      string       `json:"path"`
	Version   string       `json:"version"`
	FormCount int          `json:"form_count"`
	Forms     []Template   `json:"forms"`
	Folders   []FormFolder `json:"folders"`
}

// UpdateResponse is the API response format for the upgrade forms endpoint
type UpdateResponse struct {
	NewestVersion string `json:"newestVersion"`
	Action        string `json:"action"`
}

var client = httpClient{http.Client{Timeout: 10 * time.Second}}

// NewManager instantiates the forms manager
func NewManager(conf Config) *Manager {
	_ = os.MkdirAll(conf.FormsPath, 0o755)
	retval := &Manager{
		config: conf,
	}
	retval.postedFormData.m = make(map[string]Message)
	return retval
}

// GetFormsCatalogHandler reads all forms from config.FormsPath and writes them in the http response as a JSON object graph
// This lets the frontend present a tree-like GUI for the user to select a form for composing a message
func (m *Manager) GetFormsCatalogHandler(w http.ResponseWriter, r *http.Request) {
	formFolder, err := m.buildFormFolder()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Printf("%s %s: %s", r.Method, r.URL.Path, err)
		return
	}
	_ = json.NewEncoder(w).Encode(formFolder)
}

// PostFormDataHandler - When the user is done filling a form, the frontend posts the input fields to this handler,
// which stores them in a map, so that other browser tabs can read the values back with GetFormDataHandler
func (m *Manager) PostFormDataHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10e6); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	composeReply, _ := strconv.ParseBool(r.URL.Query().Get("composereply"))
	templatePath := r.URL.Query().Get("template")
	if templatePath == "" {
		http.Error(w, "template query param missing", http.StatusBadRequest)
		log.Printf("template query param missing %s %s", r.Method, r.URL.Path)
		return
	}
	templatePath = m.abs(templatePath)
	// Make sure we don't escape FormsPath
	if !directories.IsInPath(m.config.FormsPath, templatePath) {
		http.Error(w, fmt.Sprintf("%s escapes forms directory", templatePath), http.StatusForbidden)
		return
	}

	formInstanceKey, err := r.Cookie("forminstance")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Printf("missing cookie %s %s", templatePath, r.URL)
		return
	}
	fields := make(map[string]string, len(r.PostForm))
	for key, values := range r.PostForm {
		fields[strings.TrimSpace(strings.ToLower(key))] = values[0]
	}

	template, err := readTemplate(templatePath, formFilesFromPath(m.config.FormsPath))
	switch {
	case os.IsNotExist(err):
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("failed to parse relevant form template (%q): %v", m.rel(templatePath), err)
		return
	}
	msg, err := messageBuilder{
		Template:    template,
		FormValues:  fields,
		Interactive: false,
		IsReply:     composeReply,
		FormsMgr:    m,
	}.build()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Printf("%s %s: %s", r.Method, r.URL.Path, err)
	}

	m.postedFormData.mu.Lock()
	m.postedFormData.m[formInstanceKey.Value] = msg
	m.postedFormData.mu.Unlock()

	m.cleanupOldFormData()
	_, _ = io.WriteString(w, "<script>window.close()</script>")
}

// GetFormDataHandler is the counterpart to PostFormDataHandler. Returns the form field values to the frontend
func (m *Manager) GetFormDataHandler(w http.ResponseWriter, r *http.Request) {
	formInstanceKey, err := r.Cookie("forminstance")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Printf("missing cookie %s %s", formInstanceKey, r.URL)
		return
	}
	v, ok := m.GetPostedFormData(formInstanceKey.Value)
	if !ok {
		http.NotFound(w, r)
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

// GetPostedFormData is similar to GetFormDataHandler, but used when posting the form-based message to the outbox
func (m *Manager) GetPostedFormData(key string) (Message, bool) {
	m.postedFormData.mu.RLock()
	defer m.postedFormData.mu.RUnlock()
	v, ok := m.postedFormData.m[key]
	return v, ok
}

// GetFormTemplateHandler serves a template's HTML form (filled-in with instance values)
func (m *Manager) GetFormTemplateHandler(w http.ResponseWriter, r *http.Request) {
	templatePath := r.URL.Query().Get("template")
	if templatePath == "" {
		http.Error(w, "template query param missing", http.StatusBadRequest)
		log.Printf("template query param missing %s %s", r.Method, r.URL.Path)
		return
	}
	templatePath = m.abs(templatePath)
	// Make sure we don't escape FormsPath
	if !directories.IsInPath(m.config.FormsPath, templatePath) {
		http.Error(w, fmt.Sprintf("%s escapes forms directory", templatePath), http.StatusForbidden)
		return
	}

	template, err := readTemplate(templatePath, formFilesFromPath(m.config.FormsPath))
	switch {
	case os.IsNotExist(err):
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("failed to parse requested template (%q): %v", m.rel(templatePath), err)
		return
	}
	formPath := template.InputFormPath
	if formPath == "" {
		http.Error(w, "requested template does not provide a HTML form", http.StatusNotFound)
		return
	}

	responseText, err := m.fillFormTemplate(formPath, "/api/form?"+r.URL.Query().Encode(), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Printf("problem filling form template file %s %s: can't open template %s. Err: %s", r.Method, r.URL.Path, formPath, err)
		return
	}
	_, err = io.WriteString(w, responseText)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Printf("can't write form into response %s %s: %s", r.Method, r.URL.Path, err)
		return
	}
}

// UpdateFormTemplatesHandler handles API calls to update form templates.
func (m *Manager) UpdateFormTemplatesHandler(w http.ResponseWriter, r *http.Request) {
	response, err := m.UpdateFormTemplates(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
		return
	}
	jsn, _ := json.Marshal(response)
	_, _ = w.Write(jsn)
}

// UpdateFormTemplates handles searching for and installing the latest version of the form templates.
func (m *Manager) UpdateFormTemplates(ctx context.Context) (UpdateResponse, error) {
	if err := os.MkdirAll(m.config.FormsPath, 0o755); err != nil {
		return UpdateResponse{}, fmt.Errorf("can't write to forms dir [%w]", err)
	}
	log.Printf("Updating form templates; current version is %v", m.getFormsVersion())
	latest, err := m.getLatestFormsInfo(ctx)
	if err != nil {
		return UpdateResponse{}, err
	}
	if !m.isNewerVersion(latest.Version) {
		log.Printf("Latest forms version is %v; nothing to do", latest.Version)
		return UpdateResponse{
			NewestVersion: latest.Version,
			Action:        "none",
		}, nil
	}

	if err = m.downloadAndUnzipForms(ctx, latest.ArchiveURL); err != nil {
		return UpdateResponse{}, err
	}
	log.Printf("Finished forms update to %v", latest.Version)
	// TODO: re-init forms manager
	return UpdateResponse{
		NewestVersion: latest.Version,
		Action:        "update",
	}, nil
}

type formsInfo struct {
	Version    string `json:"version"`
	ArchiveURL string `json:"archive_url"`
}

func (m *Manager) getLatestFormsInfo(ctx context.Context) (*formsInfo, error) {
	resp, err := client.Get(ctx, m.config.UserAgent, formsVersionInfoURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("can't fetch winlink forms version page: %w", err)
	}
	defer resp.Body.Close()

	var v formsInfo
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (m *Manager) downloadAndUnzipForms(ctx context.Context, downloadLink string) error {
	log.Printf("Updating forms via %v", downloadLink)
	resp, err := client.Get(ctx, m.config.UserAgent, downloadLink)
	if err != nil {
		return fmt.Errorf("can't download update ZIP: %w", err)
	}
	defer resp.Body.Close()
	f, err := ioutil.TempFile(os.TempDir(), "pat")
	if err != nil {
		return fmt.Errorf("can't create temp file for download: %w", err)
	}
	defer f.Close()
	defer os.Remove(f.Name())
	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("can't write update ZIP: %w", err)
	}

	if err := unzip(f.Name(), m.config.FormsPath); err != nil {
		return fmt.Errorf("can't unzip forms update: %w", err)
	}
	return nil
}

// RenderForm finds the associated form and returns the filled-in form in HTML given the contents of a form attachment
func (m *Manager) RenderForm(data []byte, composeReply bool) (string, error) {
	type Node struct {
		XMLName xml.Name
		Content []byte `xml:",innerxml"`
		Nodes   []Node `xml:",any"`
	}

	data = trimBom(data)
	if !utf8.Valid(data) {
		log.Println("Warning: unsupported string encoding in form XML, expected UTF-8")
	}

	var n1 Node
	formParams := make(map[string]string)
	formVars := make(map[string]string)

	if err := xml.Unmarshal(data, &n1); err != nil {
		return "", err
	}

	if n1.XMLName.Local != "RMS_Express_Form" {
		return "", errors.New("missing RMS_Express_Form tag in form XML")
	}
	for _, n2 := range n1.Nodes {
		switch n2.XMLName.Local {
		case "form_parameters":
			for _, n3 := range n2.Nodes {
				formParams[n3.XMLName.Local] = string(n3.Content)
			}
		case "variables":
			for _, n3 := range n2.Nodes {
				formVars[n3.XMLName.Local] = string(n3.Content)
			}
		}
	}

	filesMap := formFilesFromPath(m.config.FormsPath)
	switch {
	case composeReply:
		replyTemplate := formParams["reply_template"]
		if replyTemplate == "" {
			return "", errors.New("missing reply_template tag in form XML for a reply message")
		}
		if filepath.Ext(replyTemplate) == "" {
			replyTemplate += replyFileExt
		}
		path := filesMap.get(replyTemplate)
		if path == "" {
			return "", fmt.Errorf("reply template not found: %q", replyTemplate)
		}
		template, err := readTemplate(path, filesMap)
		if err != nil {
			return "", fmt.Errorf("failed to read referenced reply template: %w", err)
		}
		submitURL := "/api/form?composereply=true&template=" + url.QueryEscape(m.rel(template.Path))
		return m.fillFormTemplate(template.InputFormPath, submitURL, formVars)
	default:
		displayForm := formParams["display_form"]
		if displayForm == "" {
			return "", errors.New("missing display_form tag in form XML")
		}
		if filepath.Ext(displayForm) == "" {
			displayForm += htmlFileExt
		}
		// Viewing a form (initial or reply)
		path := filesMap.get(displayForm)
		if path == "" {
			return "", fmt.Errorf("display from not found: %q", displayForm)
		}
		return m.fillFormTemplate(path, "", formVars)
	}
}

// ComposeTemplate composes a message from a template (templatePath) by prompting the user through stdio.
//
// It combines all data needed for the whole template-based message: subject, body, and attachments.
func (m *Manager) ComposeTemplate(templatePath string, subject string) (Message, error) {
	template, err := readTemplate(templatePath, formFilesFromPath(m.config.FormsPath))
	if err != nil {
		return Message{}, err
	}

	formValues := map[string]string{
		"subjectline":     subject,
		"templateversion": m.getFormsVersion(),
	}
	fmt.Printf("Form '%s', version: %s\n", m.rel(template.Path), formValues["templateversion"])
	return messageBuilder{
		Template:    template,
		FormValues:  formValues,
		Interactive: true,
		FormsMgr:    m,
	}.build()
}

func (m *Manager) buildFormFolder() (FormFolder, error) {
	formFolder, err := m.innerRecursiveBuildFormFolder(m.config.FormsPath, formFilesFromPath(m.config.FormsPath))
	formFolder.Version = m.getFormsVersion()
	return formFolder, err
}

func (m *Manager) innerRecursiveBuildFormFolder(rootPath string, filesMap formFilesMap) (FormFolder, error) {
	rootPath = filepath.Clean(rootPath)
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return FormFolder{}, err
	}

	folder := FormFolder{
		Name:    filepath.Base(rootPath),
		Path:    rootPath,
		Forms:   []Template{},
		Folders: []FormFolder{},
	}
	for _, entry := range entries {
		if entry.IsDir() {
			path := filepath.Join(rootPath, entry.Name())
			subfolder, err := m.innerRecursiveBuildFormFolder(path, filesMap)
			if err != nil {
				return folder, err
			}
			folder.Folders = append(folder.Folders, subfolder)
			folder.FormCount += subfolder.FormCount
			continue
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), txtFileExt) {
			continue
		}
		path := filepath.Join(rootPath, entry.Name())
		tmpl, err := readTemplate(path, filesMap)
		if err != nil {
			debug.Printf("failed to load form file %q: %v", path, err)
			continue
		}
		tmpl.Path = m.rel(tmpl.Path)
		if tmpl.InputFormPath != "" || tmpl.DisplayFormPath != "" {
			folder.Forms = append(folder.Forms, tmpl)
			folder.FormCount++
		}
	}
	sort.Slice(folder.Folders, func(i, j int) bool {
		return folder.Folders[i].Name < folder.Folders[j].Name
	})
	sort.Slice(folder.Forms, func(i, j int) bool {
		return folder.Forms[i].Name < folder.Forms[j].Name
	})
	return folder, nil
}

// abs returns the absolute path of a path relative to m.FormsPath.
//
// It is primarily used to resolve template references from the web gui, which
// are relative to m.config.FormsPath.
func (m *Manager) abs(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(m.config.FormsPath, path)
}

// rel returns a path relative to m.FormsPath.
//
// The web gui uses this variant to reference template files.
func (m *Manager) rel(path string) string {
	if !filepath.IsAbs(path) {
		return path
	}
	rel, err := filepath.Rel(m.config.FormsPath, path)
	if err != nil {
		panic(err)
	}
	return rel
}

const gpsMockAddr = "mock" // Hack for unit testing

// gpsPos returns the current GPS Position
func (m *Manager) gpsPos() (gpsd.Position, error) {
	addr := m.config.GPSd.Addr
	if addr == "" {
		return gpsd.Position{}, errors.New("GPSd: not configured.")
	}
	if addr == gpsMockAddr {
		return gpsd.Position{Lat: 59.41378, Lon: 5.268}, nil
	}
	if !m.config.GPSd.AllowForms {
		return gpsd.Position{}, errors.New("GPSd: allow_forms is disabled. GPS position will not be available in form templates.")
	}

	conn, err := gpsd.Dial(addr)
	if err != nil {
		log.Printf("GPSd daemon: %s", err)
		return gpsd.Position{}, err
	}
	defer conn.Close()

	conn.Watch(true)
	log.Println("Waiting for position from GPSd...")
	// TODO: make the GPSd timeout configurable
	return conn.NextPosTimeout(3 * time.Second)
}

func (m *Manager) fillFormTemplate(templatePath string, formDestURL string, formVars map[string]string) (string, error) {
	data, err := readFile(templatePath)
	if err != nil {
		return "", err
	}

	// Set the "form server" URL
	data = strings.ReplaceAll(data, "http://{FormServer}:{FormPort}", formDestURL)
	data = strings.ReplaceAll(data, "http://localhost:8001", formDestURL) // Some Canada BC forms are hardcoded to this URL

	// Substitute insertion tags and variables
	data = insertionTagReplacer(m, "{", "}")(data)
	data = variableReplacer("{", "}", formVars)(data)

	return data, nil
}

func (m *Manager) getFormsVersion() string {
	str, err := readFile(m.abs("Standard_Forms_Version.dat"))
	if err != nil {
		debug.Printf("failed to open version file: %v", err)
		return "unknown"
	}
	return strings.TrimSpace(str)
}

func (m *Manager) cleanupOldFormData() {
	m.postedFormData.mu.Lock()
	defer m.postedFormData.mu.Unlock()
	for key, form := range m.postedFormData.m {
		elapsed := time.Since(form.submitted).Hours()
		if elapsed > 24 {
			log.Println("deleting old FormData after", elapsed, "hrs")
			delete(m.postedFormData.m, key)
		}
	}
}

func (m *Manager) isNewerVersion(newestVersion string) bool {
	currentVersion := m.getFormsVersion()
	cv := strings.Split(currentVersion, ".")
	nv := strings.Split(newestVersion, ".")
	for i := 0; i < 4; i++ {
		var cp int64
		if len(cv) > i {
			cp, _ = strconv.ParseInt(cv[i], 10, 16)
		}
		var np int64
		if len(nv) > i {
			np, _ = strconv.ParseInt(nv[i], 10, 16)
		}
		if cp < np {
			return true
		}
	}
	return false
}

type httpClient struct{ http.Client }

func (c httpClient) Get(ctx context.Context, userAgent, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Cache-Control", "no-cache")
	return c.Do(req)
}
