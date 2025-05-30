package forms

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/textproto"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/la5nta/wl2k-go/fbb"

	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/pat/internal/editor"
)

// Message represents a concrete message compiled from a template
type Message struct {
	To          string      `json:"msg_to"`
	Cc          string      `json:"msg_cc"`
	Subject     string      `json:"msg_subject"`
	Body        string      `json:"msg_body"`
	Attachments []*fbb.File `json:"-"`

	submitted time.Time
}

type messageBuilder struct {
	Interactive     bool
	InReplyToMsg    *fbb.Message
	Template        Template
	FormValues      map[string]string
	PromptResponses map[string]string
	FormsMgr        *Manager
}

// build returns message subject, body, and attachments for the given template and variable map
func (b messageBuilder) build() (Message, error) {
	b.setDefaultFormValues()
	msg, err := b.scanAndBuild(b.Template.Path)
	if err != nil {
		return Message{}, err
	}
	msg.Attachments = b.buildAttachments()
	return msg, nil
}

// TODO: What are these "default form vars"? It looks to be a subset of the
// official insertion tags, but there is no mention of these special vars in
// the forms documentation. Consider doing insertion tag replacement with the
// {var ...} pattern instead of this.
func (b messageBuilder) setDefaultFormValues() {
	if b.InReplyToMsg != nil {
		b.FormValues["msgisreply"] = "True"

		// Here be dragons.
		// Templates using this has a strange `Def: MsgOrignalBody=<var MsgOriginalBody>`.
		// Maybe to force the inclusion of the original body in the XML? But
		// why is it referenced as a variable and not the officially supported
		// tag (i.e. `Def: MsgOriginalBody=<MsgOriginalBody>`)?
		if _, ok := b.FormValues["msgoriginalbody"]; !ok {
			b.FormValues["msgoriginalbody"], _ = b.InReplyToMsg.Body()
		}
	} else {
		b.FormValues["msgisreply"] = "False"
	}
	for _, key := range []string{"msgsender"} {
		if _, ok := b.FormValues[key]; !ok {
			b.FormValues[key] = b.FormsMgr.config.MyCall
		}
	}

	// some defaults that we can't set yet. Winlink doesn't seem to care about these
	// Set only if they're not set by form values.
	for _, key := range []string{"msgto", "msgcc", "msgsubject", "msgbody", "msgp2p", "txtstr"} {
		if _, ok := b.FormValues[key]; !ok {
			b.FormValues[key] = ""
		}
	}
	for _, key := range []string{"msgisforward", "msgisacknowledgement"} {
		if _, ok := b.FormValues[key]; !ok {
			b.FormValues[key] = "False"
		}
	}

	// TODO: Implement sequences
	for _, key := range []string{"msgseqnum"} {
		if _, ok := b.FormValues[key]; !ok {
			b.FormValues[key] = "0"
		}
	}
}

func (b messageBuilder) buildXML() []byte {
	type Variable struct {
		XMLName xml.Name
		Value   string `xml:",chardata"`
	}

	filename := func(path string) string {
		// Avoid "." for empty paths
		if path == "" {
			return ""
		}
		return filepath.Base(path)
	}

	form := struct {
		XMLName            xml.Name   `xml:"RMS_Express_Form"`
		XMLFileVersion     string     `xml:"form_parameters>xml_file_version"`
		RMSExpressVersion  string     `xml:"form_parameters>rms_express_version"`
		SubmissionDatetime string     `xml:"form_parameters>submission_datetime"`
		SendersCallsign    string     `xml:"form_parameters>senders_callsign"`
		GridSquare         string     `xml:"form_parameters>grid_square"`
		DisplayForm        string     `xml:"form_parameters>display_form"`
		ReplyTemplate      string     `xml:"form_parameters>reply_template"`
		Variables          []Variable `xml:"variables>name"`
	}{
		XMLFileVersion:     "1.0",
		RMSExpressVersion:  b.FormsMgr.config.AppVersion,
		SubmissionDatetime: now().UTC().Format("20060102150405"),
		SendersCallsign:    b.FormsMgr.config.MyCall,
		GridSquare:         b.FormsMgr.config.Locator,
		DisplayForm:        filename(b.Template.DisplayFormPath),
		ReplyTemplate:      filename(b.Template.ReplyTemplatePath),
	}
	for k, v := range b.FormValues {
		// Trim leading and trailing whitespace. Winlink Express does
		// this, judging from the produced XML attachments.
		v = strings.TrimSpace(v)
		form.Variables = append(form.Variables, Variable{xml.Name{Local: k}, v})
	}
	// Sort vars by name to make sure the output is deterministic.
	sort.Slice(form.Variables, func(i, j int) bool {
		a, b := form.Variables[i], form.Variables[j]
		return a.XMLName.Local < b.XMLName.Local
	})

	data, err := xml.MarshalIndent(form, "", "    ")
	if err != nil {
		panic(err)
	}
	return append([]byte(xml.Header), data...)
}

func (b messageBuilder) buildAttachments() []*fbb.File {
	var attachments []*fbb.File
	// Add optional text attachments defined by some forms as form values
	// pairs in the format attached_textN/attached_fileN (N=0 is omitted).
	for k := range b.FormValues {
		if !strings.HasPrefix(k, "attached_text") {
			continue
		}
		if strings.TrimSpace(b.FormValues[k]) == "" {
			// Some forms set this key as empty, meaning no real attachment.
			debug.Printf("Ignoring empty text attachment %q: %q", k, b.FormValues[k])
			continue
		}
		textKey := k
		text := b.FormValues[textKey]
		nameKey := strings.Replace(k, "attached_text", "attached_file", 1)
		name := strings.TrimSpace(b.FormValues[nameKey])
		if name == "" {
			debug.Printf("%s defined, but corresponding filename element %q is not set", textKey, nameKey)
			name = "FormData.txt" // Fallback (better than nothing)
		}
		attachments = append(attachments, fbb.NewFile(name, []byte(text)))
		delete(b.FormValues, nameKey)
		delete(b.FormValues, textKey)
	}
	// Add XML if a viewer is defined for this template
	if b.Template.DisplayFormPath != "" {
		filename := xmlName(b.Template)
		attachments = append(attachments, fbb.NewFile(filename, b.buildXML()))
	}
	return attachments
}

// scanAndBuild scans the template at the given path, applies placeholder substition and builds the message.
//
// If b,Interactive is true, the user is prompted for undefined placeholders via stdio.
func (b messageBuilder) scanAndBuild(path string) (Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return Message{}, err
	}
	defer f.Close()

	replaceInsertionTags := insertionTagReplacer(b.FormsMgr, b.InReplyToMsg, path, "<", ">")
	refreshInsertionTags := func() {
		replaceInsertionTags = insertionTagReplacer(b.FormsMgr, b.InReplyToMsg, path, "<", ">")
	}
	replaceVars := variableReplacer("<", ">", b.FormValues)
	addFormValue := func(k, v string) {
		b.FormValues[strings.ToLower(k)] = v
		replaceVars = variableReplacer("<", ">", b.FormValues) // Refresh variableReplacer (rebuild regular expressions)
		debug.Printf("Defined %q=%q", k, v)
	}

	scanner := bufio.NewScanner(newTrimBomReader(f))

	msg := Message{submitted: now()}
	var inBody bool
	for scanner.Scan() {
		lineTmpl := scanner.Text()

		// Insertion tags and variables
		lineTmpl = replaceInsertionTags(lineTmpl)
		lineTmpl = replaceVars(lineTmpl)

		// Prompt responses already provided (from text template editor in frontend)
		for search, replace := range b.PromptResponses {
			lineTmpl = strings.Replace(lineTmpl, search, replace, 1)
		}

		// Prompts (mostly found in text templates)
		if b.Interactive {
			lineTmpl = promptAsks(lineTmpl, func(a Ask) string {
				var ans string
				if a.Multiline {
					fmt.Println(a.Prompt + " (Press ENTER to start external editor)")
					b.FormsMgr.config.LineReader()
					var err error
					ans, err = editor.EditText("")
					if err != nil {
						log.Fatalf("Failed to start text editor: %v", err)
					}
				} else {
					fmt.Printf(a.Prompt + " ")
					ans = b.FormsMgr.config.LineReader()
				}
				if a.Uppercase {
					ans = strings.ToUpper(ans)
				}
				return ans
			})
			lineTmpl = promptSelects(lineTmpl, func(s Select) Option {
				for {
					fmt.Println(s.Prompt)
					for i, opt := range s.Options {
						fmt.Printf("  %d\t%s\n", i, opt.Item)
					}
					fmt.Printf("select 0-%d: ", len(s.Options)-1)
					idx, err := strconv.Atoi(b.FormsMgr.config.LineReader())
					if err == nil && idx < len(s.Options) {
						return s.Options[idx]
					}
				}
			})
			// Fallback prompt for undefined form variables.
			// Typically these are defined by the associated HTML form, but since
			// this is CLI land we'll just prompt for the variable value.
			lineTmpl = promptVars(lineTmpl, func(key string) string {
				fmt.Println(lineTmpl)
				fmt.Printf("%s: ", key)
				value := b.FormsMgr.config.LineReader()
				addFormValue(key, value)
				return value
			})
		}

		if inBody {
			msg.Body += lineTmpl + "\n"
			continue // No control fields in body
		}

		// Control fields
		switch key, value, _ := strings.Cut(lineTmpl, ":"); textproto.CanonicalMIMEHeaderKey(key) {
		case "Msg":
			// The message body starts here. No more control fields after this.
			msg.Body += value
			inBody = true
		case "Form", "ReplyTemplate":
			// Handled elsewhere
			continue
		case "Def", "Define":
			// Def: variable=value – Define the value of a variable.
			key, value, ok := strings.Cut(value, "=")
			if !ok {
				debug.Printf("Def: without key-value pair: %q", value)
				continue
			}
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			addFormValue(key, value)
		case "Subject", "Subj":
			// Set the subject of the message
			msg.Subject = strings.TrimSpace(value)
		case "To":
			// Specify to whom the message is being sent
			msg.To = strings.TrimSpace(value)
		case "Cc":
			// Specify carbon copy addresses
			msg.Cc = strings.TrimSpace(value)
		case "Readonly":
			// Yes/No – Specify whether user can edit.
			// TODO: Disable editing of body in composer?
		case "Seqinc":
			value = strings.TrimSpace(value)
			if value == "" {
				value = "1"
			}
			incr, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				log.Printf("WARNING: failed to parse Seqinc value (%q): %v", value, err)
			}
			if _, err := b.FormsMgr.sequence.Incr(incr); err != nil {
				return Message{}, err
			}
			refreshInsertionTags()
		case "Seqset":
			value = strings.TrimSpace(value)
			num, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				log.Printf("WARNING: failed to parse Seqset value (%q): %v", value, err)
			}
			if _, err := b.FormsMgr.sequence.Set(num); err != nil {
				return Message{}, err
			}
			refreshInsertionTags()
		default:
			if strings.TrimSpace(lineTmpl) != "" {
				log.Printf("skipping unknown template line: '%q'", lineTmpl)
			}
		}
	}
	if b.InReplyToMsg != nil {
		var buf bytes.Buffer
		io.Copy(&buf, strings.NewReader(msg.Body))
		writeMessageCitation(&buf, b.InReplyToMsg)
		msg.Body = buf.String()
	}
	return msg, nil
}

func writeMessageCitation(w io.Writer, inReplyToMsg *fbb.Message) {
	fmt.Fprintf(w, "--- %s %s wrote: ---\n", inReplyToMsg.Date(), inReplyToMsg.From().Addr)
	body, _ := inReplyToMsg.Body()
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		fmt.Fprintf(w, ">%s\n", scanner.Text())
	}
}

// VariableReplacer returns a function that replaces the given key-value pairs.
func variableReplacer(tagStart, tagEnd string, vars map[string]string) func(string) string {
	return placeholderReplacer(tagStart+"Var ", tagEnd, vars)
}

// InsertionTagReplacer returns a function that replaces the fixed set of insertion tags with their corresponding values.
func insertionTagReplacer(m *Manager, inReplyToMsg *fbb.Message, templatePath string, tagStart, tagEnd string) func(string) string {
	now := now()
	validPos := "NO"
	nowPos, err := m.gpsPos()
	if err != nil {
		debug.Printf("GPSd error: %v", err)
	} else {
		validPos = "YES"
		debug.Printf("GPSd position: %s", positionFmt(signedDecimal, nowPos))
	}

	internetAvailable := "NO"
	if isInternetAvailable() {
		internetAvailable = "YES"
	}

	seqNum, err := m.sequence.Load()
	if err != nil {
		debug.Printf("Error loading sequence number: %v", err)
	}

	// This list is based on RMSE_FORMS/insertion_tags.zip (copy in docs/) as well as searching Standard Forms's templates.
	tags := map[string]string{
		"MsgSender":      m.config.MyCall,
		"Callsign":       m.config.MyCall,
		"ProgramVersion": m.config.AppVersion,

		"DateTime":  formatDateTime(now),
		"UDateTime": formatDateTimeUTC(now),
		"Date":      formatDate(now),
		"UDate":     formatDateUTC(now),
		"UDTG":      formatUDTG(now),
		"Time":      formatTime(now),
		"UTime":     formatTimeUTC(now),
		"Day":       formatDay(now, location),
		"UDay":      formatDay(now, time.UTC),

		"GPS":                positionFmt(degreeMinute, nowPos),
		"GPSValid":           validPos,
		"GPS_DECIMAL":        positionFmt(decimal, nowPos),
		"GPS_SIGNED_DECIMAL": positionFmt(signedDecimal, nowPos),
		"GridSquare":         positionFmt(gridSquare, nowPos),
		"Latitude":           fmt.Sprintf("%.4f", nowPos.Lat),
		"Longitude":          fmt.Sprintf("%.4f", nowPos.Lon),
		// No docs found for these, but they are referenced by a couple of templates in Standard Forms.
		// By reading the embedded javascript, they appear to be signed decimal.
		"GPSLatitude":  fmt.Sprintf("%.4f", nowPos.Lat),
		"GPSLongitude": fmt.Sprintf("%.4f", nowPos.Lon),

		"InternetAvailable": internetAvailable,

		"MsgIsReply":           strings.Title(strconv.FormatBool(inReplyToMsg != nil)),
		"MsgIsForward":         "False",
		"MsgIsAcknowledgement": "False",

		"SeqNum": fmt.Sprintf(m.config.SequenceFormat, seqNum),

		"FormFolder": path.Join("/api/forms/", filepath.Dir(m.rel(templatePath))),

		// TODO (other insertion tags found in Standard Forms):
		// MsgTo
		// MsgCc
		// MsgSubject
		// MsgP2P
		// Sender (only in 'ARC Forms/Disaster Receipt 6409-B Reply.0')
		// Speed  (only in 'GENERAL Forms/GPS Position Report.txt' - but not included in produced message body)
		// course (only in 'GENERAL Forms/GPS Position Report.txt' - but not included in produced message body)
		// decimal_separator
	}
	if inReplyToMsg != nil {
		tags["MsgOriginalSubject"] = inReplyToMsg.Subject()
		tags["MsgOriginalSender"] = inReplyToMsg.From().Addr
		tags["MsgOriginalBody"], _ = inReplyToMsg.Body()
		tags["MsgOriginalID"] = inReplyToMsg.MID()
		tags["MsgOriginalDate"] = formatDateTime(inReplyToMsg.Date())

		// The documentation is not clear on these. Examples does not match the description.
		tags["MsgOriginalUtcDate"] = formatDateUTC(inReplyToMsg.Date())
		tags["MsgOriginalUtcTime"] = formatTimeUTC(inReplyToMsg.Date())
		tags["MsgOriginalLocalDate"] = formatDate(inReplyToMsg.Date())
		tags["MsgOriginalLocalTime"] = formatTime(inReplyToMsg.Date())
		tags["MsgOriginalDTG"] = formatUDTG(inReplyToMsg.Date()) // Assuming UTC (as per example).

		tags["MsgOriginalSize"] = fmt.Sprint(inReplyToMsg.BodySize()) // Assuming body size.
		tags["MsgOriginalAttachmentCount"] = fmt.Sprint(len(inReplyToMsg.Files()))

		for _, f := range inReplyToMsg.Files() {
			if strings.HasPrefix(f.Name(), "RMS_Express_Form_") && strings.HasSuffix(f.Name(), ".xml") {
				tags["MsgOriginalXML"] = string(f.Data())
			}
		}
	}

	return placeholderReplacer(tagStart, tagEnd, tags)
}

// xmlName returns the user-visible filename for the message attachment that holds the form instance values
func xmlName(t Template) string {
	attachmentName := filepath.Base(t.DisplayFormPath)
	attachmentName = strings.TrimSuffix(attachmentName, filepath.Ext(attachmentName))
	attachmentName = "RMS_Express_Form_" + attachmentName + ".xml"
	if len(attachmentName) > 255 {
		attachmentName = strings.TrimPrefix(attachmentName, "RMS_Express_Form_")
	}
	return attachmentName
}

func isInternetAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "HEAD", "https://www.google.com", nil)
	resp, err := http.DefaultClient.Do(req)
	debug.Printf("Internet available: %v (%v)", err == nil, err)
	if err != nil {
		return false
	}
	// Be nice, read the response body and close it.
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return true
}
