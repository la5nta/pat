package forms

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"log"
	"net/textproto"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/wl2k-go/fbb"
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
	Interactive bool
	IsReply     bool
	Template    Template
	FormValues  map[string]string
	FormsMgr    *Manager
}

// build returns message subject, body, and attachments for the given template and variable map
func (b messageBuilder) build() (Message, error) {
	b.setDefaultFormValues()
	msg, err := b.scanAndBuild(b.templatePath())
	if err != nil {
		return Message{}, err
	}
	msg.Attachments = b.buildAttachments()
	return msg, nil
}

func (b messageBuilder) templatePath() string {
	path := b.Template.TxtFileURI
	if b.IsReply && b.Template.ReplyTxtFileURI != "" {
		path = b.Template.ReplyTxtFileURI
	}
	return path
}

func (b messageBuilder) setDefaultFormValues() {
	if b.IsReply {
		b.FormValues["msgisreply"] = "True"
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

	//TODO: Implement sequences
	for _, key := range []string{"msgseqnum"} {
		if _, ok := b.FormValues[key]; !ok {
			b.FormValues[key] = "0"
		}
	}
}

func (b messageBuilder) buildXML() []byte {
	viewer := b.Template.ViewerURI
	if b.IsReply && b.Template.ReplyViewerURI != "" {
		viewer = b.Template.ReplyViewerURI
	}
	// Make sure the order in stable so the output is deterministic.
	keys := make([]string, 0, len(b.FormValues))
	for k := range b.FormValues {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	formVarsAsXML := ""
	for _, k := range keys {
		formVarsAsXML += fmt.Sprintf("    <%s>%s</%s>\n", xmlEscape(k), xmlEscape(b.FormValues[k]), xmlEscape(k))
	}
	return []byte(fmt.Sprintf(`%s<RMS_Express_Form>
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
		now().UTC().Format("20060102150405"),
		b.FormsMgr.config.MyCall,
		b.FormsMgr.config.Locator,
		filepath.Base(viewer),
		filepath.Base(b.Template.ReplyTxtFileURI),
		formVarsAsXML))
}

func (b messageBuilder) buildAttachments() []*fbb.File {
	var attachments []*fbb.File

	// Add FormData.txt attachment if defined by the form
	if v, ok := b.FormValues["attached_text"]; ok {
		delete(b.FormValues, "attached_text") // Should not be included in the XML.
		attachments = append(attachments, fbb.NewFile("FormData.txt", []byte(v)))
	}

	// Add XML if a viewer is defined for this template
	if b.Template.ViewerURI != "" {
		filename := xmlName(b.Template, b.IsReply)
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

	replaceInsertionTags := insertionTagReplacer(b.FormsMgr, "<", ">")
	replaceVars := variableReplacer("<", ">", b.FormValues)
	addFormValue := func(k, v string) {
		b.FormValues[strings.ToLower(k)] = v
		replaceVars = variableReplacer("<", ">", b.FormValues) // Refresh variableReplacer (rebuild regular expressions)
		debug.Printf("Defined %q=%q", k, v)
	}

	scanner := bufio.NewScanner(f)

	msg := Message{submitted: now()}
	var inBody bool
	for scanner.Scan() {
		lineTmpl := scanner.Text()

		// Insertion tags and variables
		lineTmpl = replaceInsertionTags(lineTmpl)
		lineTmpl = replaceVars(lineTmpl)

		// Prompts (mostly found in text templates)
		if b.Interactive {
			lineTmpl = promptAsks(lineTmpl, func(a Ask) string {
				//TODO: Handle a.Multiline as we do message body
				fmt.Printf(a.Prompt + " ")
				ans := b.FormsMgr.config.LineReader()
				if a.Uppercase {
					ans = strings.ToUpper(ans)
				}
				return b.FormsMgr.config.LineReader()
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
			//TODO: Handle sequences
		default:
			if strings.TrimSpace(lineTmpl) != "" {
				log.Printf("skipping unknown template line: '%s'", lineTmpl)
			}
		}
	}
	return msg, nil
}

// VariableReplacer returns a function that replaces the given key-value pairs.
func variableReplacer(tagStart, tagEnd string, vars map[string]string) func(string) string {
	return placeholderReplacer(tagStart+"Var ", tagEnd, vars)
}

// InsertionTagReplacer returns a function that replaces the fixed set of insertion tags with their corresponding values.
func insertionTagReplacer(m *Manager, tagStart, tagEnd string) func(string) string {
	now := now()
	validPos := "NO"
	nowPos, err := m.gpsPos()
	if err != nil {
		debug.Printf("GPSd error: %v", err)
	} else {
		validPos = "YES"
		debug.Printf("GPSd position: %s", positionFmt(signedDecimal, nowPos))
	}
	// This list is based on RMSE_FORMS/insertion_tags.zip (copy in docs/) as well as searching Standard Forms's templates.
	return placeholderReplacer(tagStart, tagEnd, map[string]string{
		"MsgSender":      m.config.MyCall,
		"Callsign":       m.config.MyCall,
		"ProgramVersion": "Pat " + m.config.AppVersion,

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
		"GPS_DECIMAL":        positionFmt(decimal, nowPos),
		"GPS_SIGNED_DECIMAL": positionFmt(signedDecimal, nowPos),
		"GridSquare":         positionFmt(gridSquare, nowPos),
		"Latitude":           fmt.Sprintf("%.4f", nowPos.Lat),
		"Longitude":          fmt.Sprintf("%.4f", nowPos.Lon),
		// No docs found for these, but they are referenced by a couple of templates in Standard Forms.
		// By reading the embedded javascript, they appear to be signed decimal.
		"GPSLatitude":  fmt.Sprintf("%.4f", nowPos.Lat),
		"GPSLongitude": fmt.Sprintf("%.4f", nowPos.Lon),
		//TODO: Why a trailing space here?
		// Some forms also adds a whitespace in their <Var > declaration, so we end up with two trailing spaces..
		"GPSValid": fmt.Sprintf("%s ", validPos),

		//TODO (other insertion tags found in Standard Forms):
		// SeqNum
		// FormFolder
		// InternetAvailable
		// MsgTo
		// MsgCc
		// MsgSubject
		// MsgP2P
		// Sender (only in 'ARC Forms/Disaster Receipt 6409-B Reply.0')
		// Speed  (only in 'GENERAL Forms/GPS Position Report.txt' - but not included in produced message body)
		// course (only in 'GENERAL Forms/GPS Position Report.txt' - but not included in produced message body)
		// decimal_separator
	})
}

// xmlName returns the user-visible filename for the message attachment that holds the form instance values
func xmlName(t Template, isReply bool) string {
	attachmentName := filepath.Base(t.ViewerURI)
	if isReply {
		attachmentName = filepath.Base(t.ReplyViewerURI)
	}
	attachmentName = strings.TrimSuffix(attachmentName, filepath.Ext(attachmentName))
	attachmentName = "RMS_Express_Form_" + attachmentName + ".xml"
	if len(attachmentName) > 255 {
		attachmentName = strings.TrimPrefix(attachmentName, "RMS_Express_Form_")
	}
	return attachmentName
}
