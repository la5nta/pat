// Copyright 2020 Rainer Grosskopf (KI7RMJ). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// Processes Winlink-compatible message template (aka Winlink forms)

package forms

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/dimchansky/utfbom"
	"github.com/la5nta/pat/cfg"
	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/pat/internal/directories"
	"github.com/la5nta/pat/internal/gpsd"
	"github.com/pd0mz/go-maidenhead"
)

const (
	fieldValueFalseInXML = "False"
	htmlFileExt          = ".html"
	txtFileExt           = ".txt"
	formsVersionInfoURL  = "https://api.getpat.io/v1/forms/standard-templates/latest"
)

// Manager manages the forms subsystem
// When the web frontend POSTs the form template data, this map holds the POST'ed data.
// Each form composer instance renders into another browser tab, and has a unique instance cookie.
// This instance cookie is the key into the map, so that we can keep the values
// from different form authoring sessions separate from each other.
type Manager struct {
	config         Config
	postedFormData struct {
		sync.RWMutex
		internalFormDataMap map[string]FormData
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

// Form holds information about a Winlink form template
type Form struct {
	Name            string `json:"name"`
	TxtFileURI      string `json:"txt_file_uri"`
	InitialURI      string `json:"initial_uri"`
	ViewerURI       string `json:"viewer_uri"`
	ReplyTxtFileURI string `json:"reply_txt_file_uri"`
	ReplyInitialURI string `json:"reply_initial_uri"`
	ReplyViewerURI  string `json:"reply_viewer_uri"`
}

// FormFolder is a folder with forms. A tree structure with Form leaves and sub-Folder branches
type FormFolder struct {
	Name      string       `json:"name"`
	Path      string       `json:"path"`
	Version   string       `json:"version"`
	FormCount int          `json:"form_count"`
	Forms     []Form       `json:"forms"`
	Folders   []FormFolder `json:"folders"`
}

// FormData holds the instance data that define a filled-in form
type FormData struct {
	TargetForm Form              `json:"target_form"`
	Fields     map[string]string `json:"fields"`
	MsgTo      string            `json:"msg_to"`
	MsgCc      string            `json:"msg_cc"`
	MsgSubject string            `json:"msg_subject"`
	MsgBody    string            `json:"msg_body"`
	MsgXML     string            `json:"msg_xml"`
	IsReply    bool              `json:"is_reply"`
	Submitted  time.Time         `json:"submitted"`
}

// MessageForm represents a concrete form-based message
type MessageForm struct {
	To             string
	Cc             string
	Subject        string
	Body           string
	AttachmentXML  string
	AttachmentName string
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
	retval.postedFormData.internalFormDataMap = make(map[string]FormData)
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
	formPath := r.URL.Query().Get("formPath")
	if formPath == "" {
		http.Error(w, "formPath query param missing", http.StatusBadRequest)
		log.Printf("formPath query param missing %s %s", r.Method, r.URL.Path)
		return
	}

	composeReply, _ := strconv.ParseBool(r.URL.Query().Get("composereply"))

	formFolder, err := m.buildFormFolder()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Printf("%s %s: %s", r.Method, r.URL.Path, err)
		return
	}

	form, err := findFormFromURI(formPath, formFolder)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Printf("can't find form to match posted form data %s %s", formPath, r.URL)
		return
	}

	formInstanceKey, err := r.Cookie("forminstance")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Printf("missing cookie %s %s", formPath, r.URL)
		return
	}
	formData := FormData{
		IsReply:    composeReply,
		TargetForm: form,
		Fields:     make(map[string]string),
	}
	for key, values := range r.PostForm {
		formData.Fields[strings.TrimSpace(strings.ToLower(key))] = values[0]
	}

	formMsg, err := formMessageBuilder{
		Template:    form,
		FormValues:  formData.Fields,
		Interactive: false,
		IsReply:     composeReply,
		FormsMgr:    m,
	}.build()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Printf("%s %s: %s", r.Method, r.URL.Path, err)
	}
	formData.MsgTo = formMsg.To
	formData.MsgCc = formMsg.Cc
	formData.MsgSubject = formMsg.Subject
	formData.MsgBody = formMsg.Body
	formData.MsgXML = formMsg.AttachmentXML
	formData.Submitted = time.Now()

	m.postedFormData.Lock()
	m.postedFormData.internalFormDataMap[formInstanceKey.Value] = formData
	m.postedFormData.Unlock()

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
	_ = json.NewEncoder(w).Encode(m.GetPostedFormData(formInstanceKey.Value))
}

// GetPostedFormData is similar to GetFormDataHandler, but used when posting the form-based message to the outbox
func (m *Manager) GetPostedFormData(key string) FormData {
	m.postedFormData.RLock()
	defer m.postedFormData.RUnlock()
	return m.postedFormData.internalFormDataMap[key]
}

// GetFormTemplateHandler handles the request for viewing a form filled-in with instance values
func (m *Manager) GetFormTemplateHandler(w http.ResponseWriter, r *http.Request) {
	formPath := r.URL.Query().Get("formPath")
	if formPath == "" {
		http.Error(w, "formPath query param missing", http.StatusBadRequest)
		log.Printf("formPath query param missing %s %s", r.Method, r.URL.Path)
		return
	}
	formPath = m.abs(formPath)
	// Make sure we don't escape FormsPath
	if !directories.IsInPath(m.config.FormsPath, formPath) {
		http.Error(w, fmt.Sprintf("%s escapes forms directory", formPath), http.StatusForbidden)
		return
	}

	responseText, err := m.fillFormTemplate(formPath, "/api/form?"+r.URL.Query().Encode(), nil, make(map[string]string))
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

func unzip(srcArchivePath, dstRoot string) error {
	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(zf *zip.File) error {
		if zf.FileInfo().IsDir() {
			return nil
		}
		destPath := filepath.Join(dstRoot, zf.Name)

		// Check for ZipSlip (Directory traversal)
		if !strings.HasPrefix(destPath, filepath.Clean(dstRoot)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", destPath)
		}

		// Ensure target directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("can't create target directory: %w", err)
		}

		// Write file
		src, err := zf.Open()
		if err != nil {
			return err
		}
		defer src.Close()
		dst, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, zf.Mode())
		if err != nil {
			return err
		}
		defer dst.Close()
		_, err = io.Copy(dst, src)
		return err
	}

	r, err := zip.OpenReader(srcArchivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetXMLAttachmentNameForForm returns the user-visible filename for the message attachment that holds the form instance values
func (m *Manager) GetXMLAttachmentNameForForm(f Form, isReply bool) string {
	attachmentName := filepath.Base(f.ViewerURI)
	if isReply {
		attachmentName = filepath.Base(f.ReplyViewerURI)
	}
	attachmentName = strings.TrimSuffix(attachmentName, filepath.Ext(attachmentName))
	attachmentName = "RMS_Express_Form_" + attachmentName + ".xml"
	if len(attachmentName) > 255 {
		attachmentName = strings.TrimPrefix(attachmentName, "RMS_Express_Form_")
	}
	return attachmentName
}

// RenderForm finds the associated form and returns the filled-in form in HTML given the contents of a form attachment
func (m *Manager) RenderForm(contentUnsanitized []byte, composeReply bool) (string, error) {
	type Node struct {
		XMLName xml.Name
		Content []byte `xml:",innerxml"`
		Nodes   []Node `xml:",any"`
	}

	sr := utfbom.SkipOnly(bytes.NewReader(contentUnsanitized))

	contentData, err := io.ReadAll(sr)
	if err != nil {
		return "", fmt.Errorf("error reading sanitized form xml: %w", err)
	}

	if !utf8.Valid(contentData) {
		log.Println("Warning: unsupported string encoding in form XML, expected utf-8")
	}

	var n1 Node
	formParams := make(map[string]string)
	formVars := make(map[string]string)

	if err := xml.Unmarshal(contentData, &n1); err != nil {
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

	switch {
	case formParams["display_form"] == "":
		return "", errors.New("missing display_form tag in form XML")
	case composeReply && formParams["reply_template"] == "":
		return "", errors.New("missing reply_template tag in form XML for a reply message")
	}

	formFolder, err := m.buildFormFolder()
	if err != nil {
		return "", err
	}

	formToLoad := formParams["display_form"]
	if composeReply {
		// we're authoring a reply
		formToLoad = formParams["reply_template"]
	}

	form, err := findFormFromURI(formToLoad, formFolder)
	if err != nil {
		return "", err
	}

	var tmplPath string
	switch {
	case composeReply:
		// authoring a form reply
		tmplPath = form.ReplyInitialURI
	case strings.HasSuffix(form.ReplyViewerURI, formParams["display_form"]):
		// viewing a form reply
		tmplPath = form.ReplyViewerURI
	default:
		// viewing a form
		tmplPath = form.ViewerURI
	}

	return m.fillFormTemplate(tmplPath, "/api/form?composereply=true&formPath="+m.rel(tmplPath), regexp.MustCompile(`{[vV][aA][rR]\s+(\w+)\s*}`), formVars)
}

// ComposeForm combines all data needed for the whole form-based message: subject, body, and attachment
func (m *Manager) ComposeForm(tmplPath string, subject string) (MessageForm, error) {
	form, err := buildFormFromTxt(tmplPath)
	if err != nil {
		return MessageForm{}, err
	}

	formValues := map[string]string{
		"subjectline":     subject,
		"templateversion": m.getFormsVersion(),
		"msgsender":       m.config.MyCall,
	}
	fmt.Printf("Form '%s', version: %s", form.TxtFileURI, formValues["templateversion"])
	formMsg, err := formMessageBuilder{
		Template:    form,
		FormValues:  formValues,
		Interactive: true,
		IsReply:     false,
		FormsMgr:    m,
	}.build()
	if err != nil {
		return MessageForm{}, err
	}

	return formMsg, nil
}

func (f Form) matchesName(nameToMatch string) bool {
	return f.InitialURI == nameToMatch ||
		strings.EqualFold(f.InitialURI, nameToMatch+htmlFileExt) ||
		f.ViewerURI == nameToMatch ||
		strings.EqualFold(f.ViewerURI, nameToMatch+htmlFileExt) ||
		f.ReplyInitialURI == nameToMatch ||
		f.ReplyInitialURI == nameToMatch+".0" ||
		f.ReplyViewerURI == nameToMatch ||
		f.ReplyViewerURI == nameToMatch+".0" ||
		f.TxtFileURI == nameToMatch ||
		strings.EqualFold(f.TxtFileURI, nameToMatch+txtFileExt)
}

func (f Form) containsName(partialName string) bool {
	return strings.Contains(f.InitialURI, partialName) ||
		strings.Contains(f.ViewerURI, partialName) ||
		strings.Contains(f.ReplyInitialURI, partialName) ||
		strings.Contains(f.ReplyViewerURI, partialName) ||
		strings.Contains(f.ReplyTxtFileURI, partialName) ||
		strings.Contains(f.TxtFileURI, partialName)
}

func (m *Manager) buildFormFolder() (FormFolder, error) {
	formFolder, err := m.innerRecursiveBuildFormFolder(m.config.FormsPath)
	formFolder.Version = m.getFormsVersion()
	return formFolder, err
}

func (m *Manager) innerRecursiveBuildFormFolder(rootPath string) (FormFolder, error) {
	rootFile, err := os.Open(rootPath)
	if err != nil {
		return FormFolder{}, err
	}
	defer rootFile.Close()
	rootFileInfo, _ := os.Stat(rootPath)

	if !rootFileInfo.IsDir() {
		return FormFolder{}, errors.New(rootPath + " is not a directory")
	}

	folder := FormFolder{
		Name:    rootFileInfo.Name(),
		Path:    rootFile.Name(),
		Forms:   []Form{},
		Folders: []FormFolder{},
	}

	infos, err := rootFile.Readdir(0)
	if err != nil {
		return folder, err
	}
	_ = rootFile.Close()

	formCnt := 0
	for _, info := range infos {
		if info.IsDir() {
			subfolder, err := m.innerRecursiveBuildFormFolder(filepath.Join(rootPath, info.Name()))
			if err != nil {
				return folder, err
			}
			folder.Folders = append(folder.Folders, subfolder)
			folder.FormCount += subfolder.FormCount
			continue
		}
		if !strings.EqualFold(filepath.Ext(info.Name()), txtFileExt) {
			continue
		}
		path := filepath.Join(rootPath, info.Name())
		frm, err := buildFormFromTxt(path)
		if err != nil {
			debug.Printf("failed to load form file %q: %v", path, err)
			continue
		}
		if frm.InitialURI != "" || frm.ViewerURI != "" {
			formCnt++
			folder.Forms = append(folder.Forms, frm)
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
func (m *Manager) abs(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(m.config.FormsPath, path)
}

// rel returns a path relative to m.FormsPath.
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

// resolveFileReference searches for files referenced in .txt files.
//
// If found the returned path is relative to FormsPath and bool is true, otherwise the given path is returned unmodified.
func resolveFileReference(basePath string, referencePath string) (string, bool) {
	path := filepath.Join(basePath, referencePath)
	if !directories.IsInPath(basePath, path) {
		debug.Printf("%q escapes template's base path (%q)", referencePath, basePath)
		return "", false
	}
	if _, err := os.Stat(path); err == nil {
		return path, true
	}
	// Fallback to case-insenstive search.
	// Some HTML files references in the .txt files has a different caseness than the actual filename on disk.
	//TODO: Walk basePath tree instead
	absPathTemplateFolder := filepath.Dir(path)
	entries, err := os.ReadDir(absPathTemplateFolder)
	if err != nil {
		return path, false
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.EqualFold(filepath.Base(path), name) {
			return filepath.Join(absPathTemplateFolder, name), true
		}
	}
	return path, false
}

func buildFormFromTxt(path string) (Form, error) {
	f, err := os.Open(path)
	if err != nil {
		return Form{}, err
	}
	defer f.Close()

	form := Form{
		Name:       strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		TxtFileURI: path,
	}
	scanner := bufio.NewScanner(f)
	baseURI := filepath.Dir(form.TxtFileURI)
	for scanner.Scan() {
		l := scanner.Text()
		switch {
		case strings.HasPrefix(l, "Form:"):
			// Form: <composer>,<viewer>
			files := strings.Split(strings.TrimPrefix(l, "Form:"), ",")
			// Extend to absolute paths and add missing html extension
			for i, path := range files {
				path = strings.TrimSpace(path)
				if ext := filepath.Ext(path); ext == "" {
					path += ".html"
				}
				var ok bool
				files[i], ok = resolveFileReference(baseURI, path)
				if !ok {
					debug.Printf("%s: failed to resolve referenced file %q", form.TxtFileURI, path)
				}
			}
			form.InitialURI = files[0]
			if len(files) > 1 {
				form.ViewerURI = files[1]
			}
		case strings.HasPrefix(l, "ReplyTemplate:"):
			path := strings.TrimSpace(strings.TrimPrefix(l, "ReplyTemplate:"))
			// Some are missing .txt
			if filepath.Ext(path) == "" {
				path += txtFileExt
			}
			var ok bool
			path, ok = resolveFileReference(baseURI, path)
			if !ok {
				debug.Printf("%s: failed to resolve referenced reply template file %q", form.TxtFileURI, path)
				continue
			}
			tmpForm, err := buildFormFromTxt(path)
			if err != nil {
				debug.Printf("%s: failed to load referenced reply template: %v", form.TxtFileURI, err)
			}
			form.ReplyTxtFileURI = path
			form.ReplyInitialURI = tmpForm.InitialURI
			form.ReplyViewerURI = tmpForm.ViewerURI
		}
	}
	return form, err
}

func findFormFromURI(formName string, folder FormFolder) (Form, error) {
	form := Form{Name: "unknown"}
	for _, subFolder := range folder.Folders {
		form, err := findFormFromURI(formName, subFolder)
		if err == nil {
			return form, nil
		}
	}

	for _, form := range folder.Forms {
		if form.matchesName(formName) {
			return form, nil
		}
	}

	// couldn't find it by full path, so try to find match by guessing folder name
	formName = path.Join(folder.Name, formName)
	for _, form := range folder.Forms {
		if form.containsName(formName) {
			return form, nil
		}
	}
	return form, errors.New("form not found")
}

// gpsPos returns the current GPS Position
func (m *Manager) gpsPos() (gpsd.Position, error) {
	addr := m.config.GPSd.Addr
	if addr == "" {
		return gpsd.Position{}, errors.New("GPSd: not configured.")
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

type gpsStyle int

const (
	// documentation: https://www.winlink.org/sites/default/files/RMSE_FORMS/insertion_tags.zip
	signedDecimal gpsStyle = iota // 41.1234 -73.4567
	decimal                       // 46.3795N 121.5835W
	degreeMinute                  // 46-22.77N 121-35.01W
)

func gpsFmt(style gpsStyle, pos gpsd.Position) string {
	var (
		northing   string
		easting    string
		latDegrees int
		latMinutes float64
		lonDegrees int
		lonMinutes float64
	)

	noPos := gpsd.Position{}
	if pos == noPos {
		return "(Not available)"
	}
	switch style {
	case degreeMinute:
		{
			latDegrees = int(math.Trunc(math.Abs(pos.Lat)))
			latMinutes = (math.Abs(pos.Lat) - float64(latDegrees)) * 60
			lonDegrees = int(math.Trunc(math.Abs(pos.Lon)))
			lonMinutes = (math.Abs(pos.Lon) - float64(lonDegrees)) * 60
		}
		fallthrough
	case decimal:
		{
			if pos.Lat >= 0 {
				northing = "N"
			} else {
				northing = "S"
			}
			if pos.Lon >= 0 {
				easting = "E"
			} else {
				easting = "W"
			}
		}
	}

	switch style {
	case signedDecimal:
		return fmt.Sprintf("%.4f %.4f", pos.Lat, pos.Lon)
	case decimal:
		return fmt.Sprintf("%.4f%s %.4f%s", math.Abs(pos.Lat), northing, math.Abs(pos.Lon), easting)
	case degreeMinute:
		return fmt.Sprintf("%02d-%05.2f%s %03d-%05.2f%s", latDegrees, latMinutes, northing, lonDegrees, lonMinutes, easting)
	default:
		return "(Not available)"
	}
}

func posToGridSquare(pos gpsd.Position) string {
	point := maidenhead.NewPoint(pos.Lat, pos.Lon)
	gridsquare, err := point.GridSquare()
	if err != nil {
		return ""
	}
	return gridsquare
}

func (m *Manager) fillFormTemplate(tmplPath string, formDestURL string, placeholderRegEx *regexp.Regexp, formVars map[string]string) (string, error) {
	fUnsanitized, err := os.Open(tmplPath)
	if err != nil {
		return "", err
	}
	defer fUnsanitized.Close()

	// skipping over UTF-8 byte-ordering mark EFBBEF, some 3rd party templates use it
	// (e.g. Sonoma county's ICS213_v2.1_SonomaACS_TwoWay_Initial_Viewer.html)
	f := utfbom.SkipOnly(fUnsanitized)

	sanitizedFileContent, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("error reading file %s", tmplPath)
	}
	if !utf8.Valid(sanitizedFileContent) {
		log.Printf("Warning: unsupported string encoding in template %s, expected utf-8", tmplPath)
	}

	now := time.Now()
	validPos := "NO"
	nowPos, err := m.gpsPos()
	if err != nil {
		debug.Printf("GPSd error: %v", err)
	} else {
		validPos = "YES"
		debug.Printf("GPSd position: %s", gpsFmt(signedDecimal, nowPos))
	}

	var buf bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(sanitizedFileContent))
	for scanner.Scan() {
		l := scanner.Text()
		l = strings.ReplaceAll(l, "http://{FormServer}:{FormPort}", formDestURL)
		// some Canada BC forms don't use the {FormServer} placeholder, it's OK, can deal with it here
		l = strings.ReplaceAll(l, "http://localhost:8001", formDestURL)
		l = strings.ReplaceAll(l, "{MsgSender}", m.config.MyCall)
		l = strings.ReplaceAll(l, "{Callsign}", m.config.MyCall)
		l = strings.ReplaceAll(l, "{ProgramVersion}", "Pat "+m.config.AppVersion)
		l = strings.ReplaceAll(l, "{DateTime}", formatDateTime(now))
		l = strings.ReplaceAll(l, "{UDateTime}", formatDateTimeUTC(now))
		l = strings.ReplaceAll(l, "{Date}", formatDate(now))
		l = strings.ReplaceAll(l, "{UDate}", formatDateUTC(now))
		l = strings.ReplaceAll(l, "{UDTG}", formatUDTG(now))
		l = strings.ReplaceAll(l, "{Time}", formatTime(now))
		l = strings.ReplaceAll(l, "{UTime}", formatTimeUTC(now))
		l = strings.ReplaceAll(l, "{GPS}", gpsFmt(degreeMinute, nowPos))
		l = strings.ReplaceAll(l, "{GPS_DECIMAL}", gpsFmt(decimal, nowPos))
		l = strings.ReplaceAll(l, "{GPS_SIGNED_DECIMAL}", gpsFmt(signedDecimal, nowPos))
		// Lots of undocumented tags found in the Winlink check in form.
		// Note also various ways of capitalizing. Perhaps best to do case insenstive string replacements....
		l = strings.ReplaceAll(l, "{Latitude}", fmt.Sprintf("%.4f", nowPos.Lat))
		l = strings.ReplaceAll(l, "{latitude}", fmt.Sprintf("%.4f", nowPos.Lat))
		l = strings.ReplaceAll(l, "{Longitude}", fmt.Sprintf("%.4f", nowPos.Lon))
		l = strings.ReplaceAll(l, "{longitude}", fmt.Sprintf("%.4f", nowPos.Lon))
		l = strings.ReplaceAll(l, "{GridSquare}", posToGridSquare(nowPos))
		l = strings.ReplaceAll(l, "{GPSValid}", fmt.Sprintf("%s ", validPos))
		if placeholderRegEx != nil {
			l = fillPlaceholders(l, placeholderRegEx, formVars)
		}
		buf.WriteString(l + "\n")
	}
	return buf.String(), nil
}

func (m *Manager) getFormsVersion() string {
	data, err := os.ReadFile(m.abs("Standard_Forms_Version.dat"))
	if err != nil {
		debug.Printf("failed to open version file: %v", err)
		return "unknown"
	}
	return string(bytes.TrimSpace(data))
}

type formMessageBuilder struct {
	Interactive bool
	IsReply     bool
	Template    Form
	FormValues  map[string]string
	FormsMgr    *Manager
}

// build returns message subject, body, and XML attachment content for the given template and variable map
func (b formMessageBuilder) build() (MessageForm, error) {
	tmplPath := b.Template.TxtFileURI
	if b.IsReply && b.Template.ReplyTxtFileURI != "" {
		tmplPath = b.Template.ReplyTxtFileURI
	}

	b.initFormValues()

	formVarsAsXML := ""
	for varKey, varVal := range b.FormValues {
		formVarsAsXML += fmt.Sprintf("    <%s>%s</%s>\n", xmlEscape(varKey), xmlEscape(varVal), xmlEscape(varKey))
	}

	viewer := ""
	if b.Template.ViewerURI != "" {
		viewer = filepath.Base(b.Template.ViewerURI)
	}
	if b.IsReply && b.Template.ReplyViewerURI != "" {
		viewer = filepath.Base(b.Template.ReplyViewerURI)
	}

	replier := ""
	if !b.IsReply && b.Template.ReplyTxtFileURI != "" {
		replier = filepath.Base(b.Template.ReplyTxtFileURI)
	}

	msgForm, err := b.scanTmplBuildMessage(tmplPath)
	if err != nil {
		return MessageForm{}, err
	}

	// Add XML if a viewer is defined for this form
	if b.Template.ViewerURI != "" {
		msgForm.AttachmentXML = fmt.Sprintf(`%s<RMS_Express_Form>
  <form_parameters>
    <xml_file_version>%s</xml_file_version>
    <rms_express_version>%s</rms_express_version>
    <submission_datetime>%s</submission_datetime>
    <senders_callsign>%s</senders_callsign>
    <grid_square>%s</grid_square>
    <display_form>%s</display_form>
    <reply_template>%s</reply_template>
  </form_parameters>
  <variables>
%s
  </variables>
</RMS_Express_Form>
`,
			xml.Header,
			"1.0",
			b.FormsMgr.config.AppVersion,
			time.Now().UTC().Format("20060102150405"),
			b.FormsMgr.config.MyCall,
			b.FormsMgr.config.Locator,
			viewer,
			replier,
			formVarsAsXML)
		msgForm.AttachmentName = b.FormsMgr.GetXMLAttachmentNameForForm(b.Template, false)
	}

	msgForm.To = strings.TrimSpace(msgForm.To)
	msgForm.Cc = strings.TrimSpace(msgForm.Cc)
	msgForm.Subject = strings.TrimSpace(msgForm.Subject)
	msgForm.Body = strings.TrimSpace(msgForm.Body)
	return msgForm, nil
}

func (b formMessageBuilder) initFormValues() {
	if b.IsReply {
		b.FormValues["msgisreply"] = "True"
	} else {
		b.FormValues["msgisreply"] = "False"
	}

	b.FormValues["msgsender"] = b.FormsMgr.config.MyCall

	// some defaults that we can't set yet. Winlink doesn't seem to care about these
	b.FormValues["msgto"] = ""
	b.FormValues["msgcc"] = ""
	b.FormValues["msgsubject"] = ""
	b.FormValues["msgbody"] = ""
	b.FormValues["msgp2p"] = ""
	b.FormValues["msgisforward"] = fieldValueFalseInXML
	b.FormValues["msgisacknowledgement"] = fieldValueFalseInXML
	b.FormValues["msgseqnum"] = "0"
}

func (b formMessageBuilder) scanTmplBuildMessage(tmplPath string) (MessageForm, error) {
	infile, err := os.Open(tmplPath)
	if err != nil {
		return MessageForm{}, err
	}
	defer infile.Close()

	placeholderRegEx := regexp.MustCompile(`<[vV][aA][rR]\s+(\w+)\s*>`)
	scanner := bufio.NewScanner(infile)

	var msgForm MessageForm
	var inBody bool
	for scanner.Scan() {
		lineTmpl := scanner.Text()
		lineTmpl = fillPlaceholders(lineTmpl, placeholderRegEx, b.FormValues)
		lineTmpl = strings.ReplaceAll(lineTmpl, "<MsgSender>", b.FormsMgr.config.MyCall)
		lineTmpl = strings.ReplaceAll(lineTmpl, "<ProgramVersion>", "Pat "+b.FormsMgr.config.AppVersion)
		if strings.HasPrefix(lineTmpl, "Form:") {
			continue
		}
		if strings.HasPrefix(lineTmpl, "ReplyTemplate:") {
			continue
		}
		if strings.HasPrefix(lineTmpl, "Msg:") {
			lineTmpl = strings.TrimSpace(strings.TrimPrefix(lineTmpl, "Msg:"))
			inBody = true
		}
		if b.Interactive {
			matches := placeholderRegEx.FindAllStringSubmatch(lineTmpl, -1)
			fmt.Println(lineTmpl)
			for i := range matches {
				varName := matches[i][1]
				varNameLower := strings.ToLower(varName)
				if b.FormValues[varNameLower] != "" {
					continue
				}
				fmt.Print(varName + ": ")
				b.FormValues[varNameLower] = "blank"
				val := b.FormsMgr.config.LineReader()
				if val != "" {
					b.FormValues[varNameLower] = val
				}
			}
		}

		lineTmpl = fillPlaceholders(lineTmpl, placeholderRegEx, b.FormValues)
		switch {
		case strings.HasPrefix(lineTmpl, "Subject:"):
			msgForm.Subject = strings.TrimPrefix(lineTmpl, "Subject:")
		case strings.HasPrefix(lineTmpl, "To:"):
			msgForm.To = strings.TrimPrefix(lineTmpl, "To:")
		case strings.HasPrefix(lineTmpl, "Cc:"):
			msgForm.Cc = strings.TrimPrefix(lineTmpl, "Cc:")
		case inBody:
			msgForm.Body += lineTmpl + "\n"
		default:
			log.Printf("skipping unknown template line: '%s'", lineTmpl)
		}
	}
	return msgForm, nil
}

func xmlEscape(s string) string {
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(s)); err != nil {
		log.Printf("Error trying to escape XML string %s", err)
	}
	return buf.String()
}

func fillPlaceholders(s string, re *regexp.Regexp, values map[string]string) string {
	if _, ok := values["txtstr"]; !ok {
		values["txtstr"] = ""
	}
	result := s
	matches := re.FindAllStringSubmatch(s, -1)
	for _, match := range matches {
		value, ok := values[strings.ToLower(match[1])]
		if ok {
			result = strings.ReplaceAll(result, match[0], value)
		}
	}
	return result
}

func (m *Manager) cleanupOldFormData() {
	m.postedFormData.Lock()
	defer m.postedFormData.Unlock()
	for key, form := range m.postedFormData.internalFormDataMap {
		elapsed := time.Since(form.Submitted).Hours()
		if elapsed > 24 {
			log.Println("deleting old FormData after", elapsed, "hrs")
			delete(m.postedFormData.internalFormDataMap, key)
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
