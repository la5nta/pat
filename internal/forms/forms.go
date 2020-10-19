// Copyright 2020 Rainer Grosskopf (KI7RMJ). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// Processes Winlink-compatible message template (aka Winlink forms)

package forms

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
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
)

const fieldValueFalseInXML = "False"
const txtFileExt = ".txt"

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
	MsgSubject string            `json:"msg_subject"`
	MsgBody    string            `json:"msg_body"`
	MsgXML     string            `json:"msg_xml"`
	IsReply    bool              `json:"is_reply"`
	Submitted  time.Time         `json:"submitted"`
}

// MessageForm represents a concrete form-based message
type MessageForm struct {
	Subject        string
	Body           string
	AttachmentXML  string
	AttachmentName string
}

// NewManager instantiates the forms manager
func NewManager(conf Config) *Manager {
	retval := &Manager{
		config: conf,
	}
	retval.postedFormData.internalFormDataMap = make(map[string]FormData)
	return retval
}

// GetFormsCatalogHandler reads all forms from config.FormsPath and writes them in the http response as a JSON object graph
// This lets the frontend present a tree-like GUI for the user to select a form for composing a message
func (mgr *Manager) GetFormsCatalogHandler(w http.ResponseWriter, r *http.Request) {
	formFolder, err := mgr.buildFormFolder()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Printf("%s %s: %s", r.Method, r.URL.Path, err)
		return
	}
	json.NewEncoder(w).Encode(formFolder)
}

// PostFormDataHandler - When the user is done filling a form, the frontend posts the input fields to this handler,
// which stores them in a map, so that other browser tabs can read the values back with GetFormDataHandler
func (mgr *Manager) PostFormDataHandler(w http.ResponseWriter, r *http.Request) {
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

	composereply, _ := strconv.ParseBool(r.URL.Query().Get("composereply"))

	formFolder, err := mgr.buildFormFolder()
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
		IsReply:    composereply,
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
		IsReply:     composereply,
		FormsMgr:    mgr,
	}.build()

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Printf("%s %s: %s", r.Method, r.URL.Path, err)
	}
	formData.MsgSubject = formMsg.Subject
	formData.MsgBody = formMsg.Body
	formData.MsgXML = formMsg.AttachmentXML
	formData.Submitted = time.Now()

	mgr.postedFormData.Lock()
	mgr.postedFormData.internalFormDataMap[formInstanceKey.Value] = formData
	mgr.postedFormData.Unlock()

	mgr.cleanupOldFormData()
	io.WriteString(w, "<script>window.close()</script>")
}

// GetFormDataHandler is the counterpart to PostFormDataHandler. Returns the form field values to the frontend
func (mgr *Manager) GetFormDataHandler(w http.ResponseWriter, r *http.Request) {
	formInstanceKey, err := r.Cookie("forminstance")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Printf("missing cookie %s %s", formInstanceKey, r.URL)
		return
	}
	json.NewEncoder(w).Encode(mgr.GetPostedFormData(formInstanceKey.Value))
}

// GetPostedFormData is similar to GetFormDataHandler, but used when posting the form-based message to the outbox
func (mgr *Manager) GetPostedFormData(key string) FormData {
	mgr.postedFormData.RLock()
	retVal := mgr.postedFormData.internalFormDataMap[key]
	mgr.postedFormData.RUnlock()
	return retVal
}

// GetFormTemplateHandler handles the request for viewing a form filled-in with instance values
func (mgr *Manager) GetFormTemplateHandler(w http.ResponseWriter, r *http.Request) {
	formPath := r.URL.Query().Get("formPath")
	if formPath == "" {
		http.Error(w, "formPath query param missing", http.StatusBadRequest)
		log.Printf("formPath query param missing %s %s", r.Method, r.URL.Path)
		return
	}

	absPathTemplate, err := mgr.findAbsPathForTemplatePath(formPath)
	if err != nil {
		http.Error(w, "find the full path for requested template "+formPath, http.StatusBadRequest)
		log.Printf("find the full path for requested template %s %s: %s", r.Method, r.URL.Path, "can't open template "+formPath)
		return
	}

	responseText, err := mgr.fillFormTemplate(absPathTemplate, "/api/form?"+r.URL.Query().Encode(), nil, make(map[string]string))
	if err != nil {
		http.Error(w, "can't open template "+formPath, http.StatusBadRequest)
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

// GetXMLAttachmentNameForForm returns the user-visible filename for the message attachment that holds the form instance values
func (mgr *Manager) GetXMLAttachmentNameForForm(f Form, isReply bool) string {
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
func (mgr *Manager) RenderForm(contentUnsanitized []byte, composereply bool) (string, error) {

	type Node struct {
		XMLName xml.Name
		Content []byte `xml:",innerxml"`
		Nodes   []Node `xml:",any"`
	}

	sr := utfbom.SkipOnly(bytes.NewReader(contentUnsanitized))

	contentData, err := ioutil.ReadAll(sr)
	if err != nil {
		return "", fmt.Errorf("error reading sanitized form xml: %s", err)
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
	case composereply && formParams["reply_template"] == "":
		return "", errors.New("missing reply_template tag in form XML for a reply message")
	}

	formFolder, err := mgr.buildFormFolder()
	if err != nil {
		return "", err
	}

	formToLoad := formParams["display_form"]
	if composereply {
		// we're authoring a reply
		formToLoad = formParams["reply_template"]
	}

	form, err := findFormFromURI(formToLoad, formFolder)
	if err != nil {
		return "", err
	}

	var formRelPath string
	switch {
	case composereply:
		// authoring a form reply
		formRelPath = form.ReplyInitialURI
	case strings.HasSuffix(form.ReplyViewerURI, formParams["display_form"]):
		//viewing a form reply
		formRelPath = form.ReplyViewerURI
	default:
		// viewing a form
		formRelPath = form.ViewerURI
	}

	absPathTemplate, err := mgr.findAbsPathForTemplatePath(formRelPath)
	if err != nil {
		return "", err
	}

	retVal, err := mgr.fillFormTemplate(absPathTemplate, "/api/form?composereply=true&formPath="+formRelPath, regexp.MustCompile(`\{var\s+(\w+)\s*\}`), formVars)
	return retVal, err
}

// ComposeForm combines all data needed for the whole form-based message: subject, body, and attachment
func (mgr *Manager) ComposeForm(tmplPath string, subject string) (MessageForm, error) {

	formFolder, err := mgr.buildFormFolder()
	if err != nil {
		log.Printf("can't build form folder tree %s", err)
		return MessageForm{}, err
	}

	tmplPath = filepath.Clean(tmplPath)
	form, err := findFormFromURI(tmplPath, formFolder)
	if err != nil {
		log.Printf("can't find form to match form %s", tmplPath)
		return MessageForm{}, err
	}

	varMap := map[string]string{
		"subjectline":     subject,
		"templateversion": mgr.getFormsVersion(),
		"msgsender":       mgr.config.MyCall,
	}

	fmt.Printf("Form '%s', version: %s", form.TxtFileURI, varMap["templateversion"])

	formMsg, err := formMessageBuilder{
		Template:    form,
		FormValues:  varMap,
		Interactive: true,
		IsReply:     false,
		FormsMgr:    mgr,
	}.build()

	if err != nil {
		log.Printf("Could not open form file '%s'.\nRun 'pat configure' and verify that 'forms_path' is set up and the files exist.\n", tmplPath)
		return MessageForm{}, err
	}

	return formMsg, nil
}

func (f Form) matchesName(nameToMatch string) bool {
	return f.InitialURI == nameToMatch ||
		f.InitialURI == nameToMatch+".html" ||
		f.ViewerURI == nameToMatch ||
		f.ViewerURI == nameToMatch+".html" ||
		f.ReplyInitialURI == nameToMatch ||
		f.ReplyInitialURI == nameToMatch+".0" ||
		f.ReplyViewerURI == nameToMatch ||
		f.ReplyViewerURI == nameToMatch+".0" ||
		f.TxtFileURI == nameToMatch ||
		f.TxtFileURI == nameToMatch+".txt"
}

func (f Form) containsName(partialName string) bool {
	return strings.Contains(f.InitialURI, partialName) ||
		strings.Contains(f.ViewerURI, partialName) ||
		strings.Contains(f.ReplyInitialURI, partialName) ||
		strings.Contains(f.ReplyViewerURI, partialName) ||
		strings.Contains(f.ReplyTxtFileURI, partialName) ||
		strings.Contains(f.TxtFileURI, partialName)
}

func (mgr *Manager) buildFormFolder() (FormFolder, error) {
	formFolder, err := mgr.innerRecursiveBuildFormFolder(mgr.config.FormsPath)
	formFolder.Version = mgr.getFormsVersion()
	return formFolder, err
}

func (mgr *Manager) innerRecursiveBuildFormFolder(rootPath string) (FormFolder, error) {
	rootFile, err := os.Open(rootPath)
	if err != nil {
		return FormFolder{}, err
	}
	defer rootFile.Close()
	rootFileInfo, _ := os.Stat(rootPath)

	if !rootFileInfo.IsDir() {
		return FormFolder{}, errors.New(rootPath + " is not a directory")
	}

	retVal := FormFolder{
		Name:    rootFileInfo.Name(),
		Path:    rootFile.Name(),
		Forms:   []Form{},
		Folders: []FormFolder{},
	}

	infos, err := rootFile.Readdir(0)
	if err != nil {
		return retVal, err
	}
	rootFile.Close()

	formCnt := 0
	for _, info := range infos {
		if info.IsDir() {
			subfolder, err := mgr.innerRecursiveBuildFormFolder(path.Join(rootPath, info.Name()))
			if err != nil {
				return retVal, err
			}
			retVal.Folders = append(retVal.Folders, subfolder)
			retVal.FormCount += subfolder.FormCount
			continue
		}
		if filepath.Ext(info.Name()) != txtFileExt {
			continue
		}
		frm, err := mgr.buildFormFromTxt(path.Join(rootPath, info.Name()))
		if err != nil {
			continue
		}
		if frm.InitialURI != "" || frm.ViewerURI != "" {
			formCnt++
			retVal.Forms = append(retVal.Forms, frm)
			retVal.FormCount++
		}
	}
	sort.Slice(retVal.Folders, func(i, j int) bool {
		return retVal.Folders[i].Name < retVal.Folders[j].Name
	})
	sort.Slice(retVal.Forms, func(i, j int) bool {
		return retVal.Forms[i].Name < retVal.Forms[j].Name
	})
	return retVal, nil
}

func (mgr *Manager) buildFormFromTxt(txtPath string) (Form, error) {
	f, err := os.Open(txtPath)
	if err != nil {
		return Form{}, err
	}
	defer f.Close()

	formsPathWithSlash := mgr.config.FormsPath + "/"

	retVal := Form{
		Name:       strings.TrimSuffix(path.Base(txtPath), ".txt"),
		TxtFileURI: strings.TrimPrefix(txtPath, formsPathWithSlash),
	}
	scanner := bufio.NewScanner(f)
	baseURI := path.Dir(retVal.TxtFileURI)
	for scanner.Scan() {
		l := scanner.Text()
		switch {
		case strings.HasPrefix(l, "Form:"):
			trimmed := strings.TrimSpace(strings.TrimPrefix(l, "Form:"))
			fileNames := strings.Split(trimmed, ",")
			if fileNames != nil && len(fileNames) >= 2 {
				initial := strings.TrimSpace(fileNames[0])
				viewer := strings.TrimSpace(fileNames[1])
				retVal.InitialURI = path.Join(baseURI, initial)
				retVal.ViewerURI = path.Join(baseURI, viewer)
			}
		case strings.HasPrefix(l, "ReplyTemplate:"):
			retVal.ReplyTxtFileURI = path.Join(baseURI, strings.TrimSpace(strings.TrimPrefix(l, "ReplyTemplate:")))
			tmpForm, _ := mgr.buildFormFromTxt(path.Join(mgr.config.FormsPath, retVal.ReplyTxtFileURI))
			retVal.ReplyInitialURI = tmpForm.InitialURI
			retVal.ReplyViewerURI = tmpForm.ViewerURI
		}
	}
	return retVal, err
}

func findFormFromURI(formName string, folder FormFolder) (Form, error) {
	retVal := Form{Name: "unknown"}
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
	return retVal, errors.New("form not found")
}

func (mgr *Manager) findAbsPathForTemplatePath(tmplPath string) (string, error) {
	absPathTemplate := filepath.Join(mgr.config.FormsPath, path.Clean(tmplPath))

	// now deal with cases where the html file name specified in the .txt file, has different caseness than the actual .html file on disk.
	absPathTemplateFolder := filepath.Dir(absPathTemplate)

	templateDir, err := os.Open(absPathTemplateFolder)
	if err != nil {
		return "", errors.New("can't read template folder")
	}
	defer templateDir.Close()

	fileNames, err := templateDir.Readdirnames(0)
	if err != nil {
		return "", errors.New("can't read template folder")
	}

	var retVal string
	for _, name := range fileNames {
		if strings.ToLower(filepath.Base(tmplPath)) == strings.ToLower(name) {
			retVal = filepath.Join(absPathTemplateFolder, name)
			break
		}
	}

	return retVal, nil
}

func (mgr *Manager) fillFormTemplate(absPathTemplate string, formDestURL string, placeholderRegEx *regexp.Regexp, formVars map[string]string) (string, error) {
	fUnsanitized, err := os.Open(absPathTemplate)
	if err != nil {
		return "", err
	}
	defer fUnsanitized.Close()

	// skipping over UTF-8 byte-ordering mark EFBBEF, some 3rd party templates use it
	// (e.g. Sonoma county's ICS213_v2.1_SonomaACS_TwoWay_Initial_Viewer.html)
	f := utfbom.SkipOnly(fUnsanitized)

	sanitizedFileContent, err := ioutil.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("error reading file %s", absPathTemplate)
	}
	if !utf8.Valid(sanitizedFileContent) {
		log.Printf("Warning: unsupported string encoding in template %s, expected utf-8", absPathTemplate)
	}

	retVal := ""
	now := time.Now()
	nowDateTime := now.Format("2006-01-02 15:04:05")
	nowDateTimeUTC := now.UTC().Format("2006-01-02 15:04:05Z")
	nowDate := now.Format("2006-01-02")
	nowTime := now.Format("15:04:05")
	nowDateUTC := now.UTC().Format("2006-01-02Z")
	nowTimeUTC := now.UTC().Format("15:04:05Z")
	udtg := strings.ToUpper(now.UTC().Format("021504Z Jan 2006"))

	scanner := bufio.NewScanner(bytes.NewReader(sanitizedFileContent))
	for scanner.Scan() {
		l := scanner.Text()
		l = strings.Replace(l, "http://{FormServer}:{FormPort}", formDestURL, -1)
		// some Canada BC forms don't use the {FormServer} placeholder, it's OK, can deal with it here
		l = strings.Replace(l, "http://localhost:8001", formDestURL, -1)
		l = strings.Replace(l, "{MsgSender}", mgr.config.MyCall, -1)
		l = strings.Replace(l, "{Callsign}", mgr.config.MyCall, -1)
		l = strings.Replace(l, "{ProgramVersion}", "Pat "+mgr.config.AppVersion, -1)
		l = strings.Replace(l, "{DateTime}", nowDateTime, -1)
		l = strings.Replace(l, "{UDateTime}", nowDateTimeUTC, -1)
		l = strings.Replace(l, "{Date}", nowDate, -1)
		l = strings.Replace(l, "{UDate}", nowDateUTC, -1)
		l = strings.Replace(l, "{UDTG}", udtg, -1)
		l = strings.Replace(l, "{Time}", nowTime, -1)
		l = strings.Replace(l, "{UTime}", nowTimeUTC, -1)
		if placeholderRegEx != nil {
			l = fillPlaceholders(l, placeholderRegEx, formVars)
		}
		retVal += l + "\n"
	}
	return retVal, nil
}

func (mgr *Manager) getFormsVersion() string {
	// walking up the path to find a version file.
	// Winlink's Standard_Forms.zip includes it in its root.
	dir := mgr.config.FormsPath
	if filepath.Ext(dir) == txtFileExt {
		dir = filepath.Dir(dir)
	}

	var verFile *os.File
	// loop to walk up the subfolders until we find the top, or Winlink's Standard_Forms_Version.dat file
	for {
		f, err := os.Open(filepath.Join(dir, "Standard_Forms_Version.dat"))
		if err != nil {
			dir = filepath.Dir(dir) // have not found the version file or couldn't open it, going up by one
			if dir == "." || dir == ".." || strings.HasSuffix(dir, string(os.PathSeparator)) {
				return "unknown" // reached top-level and couldn't find version .dat file
			}
			continue
		}
		defer f.Close()
		// found and opened the version file
		verFile = f
		break
	}

	if verFile != nil {
		return readFileFirstLine(verFile)
	}
	return "unknown"
}

func readFileFirstLine(f *os.File) string {
	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		return scanner.Text()
	}
	return ""
}

type formMessageBuilder struct {
	Interactive bool
	IsReply     bool
	Template    Form
	FormValues  map[string]string
	FormsMgr    *Manager
}

//build returns message subject, body, and XML attachment content for the given template and variable map
func (b formMessageBuilder) build() (MessageForm, error) {

	tmplPath := filepath.Join(b.FormsMgr.config.FormsPath, b.Template.TxtFileURI)
	if filepath.Ext(tmplPath) == "" {
		tmplPath += txtFileExt
	}
	if b.IsReply && b.Template.ReplyTxtFileURI != "" {
		tmplPath = filepath.Join(b.FormsMgr.config.FormsPath, b.Template.ReplyTxtFileURI)
	}

	retVal, err := b.scanTmplBuildMessage(tmplPath)
	if err != nil {
		return MessageForm{}, err
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

	retVal.AttachmentXML = fmt.Sprintf(`%s<RMS_Express_Form>
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
	retVal.AttachmentName = b.FormsMgr.GetXMLAttachmentNameForForm(b.Template, false)
	retVal.Subject = strings.TrimSpace(retVal.Subject)
	retVal.Body = strings.TrimSpace(retVal.Body)
	return retVal, nil
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

	var retVal MessageForm
	for scanner.Scan() {
		lineTmpl := scanner.Text()
		lineTmpl = fillPlaceholders(lineTmpl, placeholderRegEx, b.FormValues)
		lineTmpl = strings.Replace(lineTmpl, "<MsgSender>", b.FormsMgr.config.MyCall, -1)
		lineTmpl = strings.Replace(lineTmpl, "<ProgramVersion>", "Pat "+b.FormsMgr.config.AppVersion, -1)
		if strings.HasPrefix(lineTmpl, "Form:") ||
			strings.HasPrefix(lineTmpl, "ReplyTemplate:") ||
			strings.HasPrefix(lineTmpl, "To:") ||
			strings.HasPrefix(lineTmpl, "Msg:") {
			continue
		}
		if b.Interactive {
			matches := placeholderRegEx.FindAllStringSubmatch(lineTmpl, -1)
			fmt.Println(string(lineTmpl))
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
		if strings.HasPrefix(lineTmpl, "Subject:") {
			retVal.Subject = strings.TrimPrefix(lineTmpl, "Subject:")
		} else {
			retVal.Body += lineTmpl + "\n"
		}
	}

	return retVal, nil
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
			result = strings.Replace(result, match[0], value, -1)
		}
	}
	return result
}

func (mgr *Manager) cleanupOldFormData() {
	mgr.postedFormData.Lock()
	for key, form := range mgr.postedFormData.internalFormDataMap {
		elapsed := time.Now().Sub(form.Submitted).Hours()
		if elapsed > 24 {
			log.Println("deleting old FormData after", elapsed, "hrs")
			delete(mgr.postedFormData.internalFormDataMap, key)
		}
	}
	mgr.postedFormData.Unlock()
}
