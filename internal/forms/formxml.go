package forms

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"unicode/utf8"
)

// formParams holds the form_parameters element of an RMS_Express_Form
// document.
//
// The first five fields are marshalled by encoding/xml as ordinary
// character data: they are ASCII by construction (version strings, a
// callsign, a grid square, a formatted date), so the standard escaping
// suffices.
//
// The two remaining fields carry template filenames, which may contain
// non-ASCII characters (e.g. the French "Accusé de réception.txt" reply
// template). Winlink Express does not honor the XML encoding declaration:
// raw UTF-8 bytes are rendered as '?', silently dropping text and breaking
// reply templates that no longer resolve. The only way to pass such
// characters through is numeric character references — which encoding/xml
// cannot be made to emit, since it re-escapes any '&' fed to it — so the
// filename elements hold escapedXMLText values: content escaped by hand
// (see messageBuilder.buildXML) and written verbatim.
type formParams struct {
	// Marshalled by encoding/xml as ordinary character data, with
	// standard escaping.
	XMLFileVersion     string `xml:"xml_file_version"`
	RMSExpressVersion  string `xml:"rms_express_version"`
	SubmissionDatetime string `xml:"submission_datetime"`
	SendersCallsign    string `xml:"senders_callsign"`
	GridSquare         string `xml:"grid_square"`

	// Escaped manually at assembly (see messageBuilder.buildXML) and
	// written verbatim: template filenames, which may be non-ASCII and
	// must therefore be numeric character references for Winlink Express.
	DisplayForm   escapedXMLText `xml:"display_form"`
	ReplyTemplate escapedXMLText `xml:"reply_template"`
}

// escapedXMLText is XML character data that has already been escaped, in
// the spirit of json.RawMessage: encoding/xml writes its value verbatim
// (",innerxml") instead of escaping it again. Values must be produced by
// escapeXMLText, which is the only sanctioned producer.
type escapedXMLText struct {
	Value string `xml:",innerxml"`
}

// formVariable is a named form variable, marshalled as an element named
// after XMLName, with the form value as its content. The content is
// embedded from escapedXMLText: form values may be non-ASCII, and must be
// numeric character references for Winlink Express, so callers must
// pre-escape them with escapeXMLText.
type formVariable struct {
	XMLName xml.Name
	escapedXMLText
}

// escapeXMLText returns s as escapedXMLText, for use as verbatim
// (",innerxml") XML content.
//
// encoding/xml's own escaping (xml.EscapeText) handles the ASCII
// characters — including the hexadecimal forms for \t, \n and \r — but
// passes non-ASCII runes through as raw UTF-8, which Winlink Express
// renders as '?'. Numeric character references are the only way to pass
// such characters through, and encoding/xml cannot be made to emit them,
// so every rune above ASCII in the escaped text is re-encoded here as a
// reference — including the U+FFFD replacements xml.EscapeText
// substitutes for unrepresentable runes, keeping the document pure ASCII.
func escapeXMLText(s string) escapedXMLText {
	var buf bytes.Buffer
	// Writing to a bytes.Buffer never fails.
	_ = xml.EscapeText(&buf, []byte(s))
	var b strings.Builder
	b.Grow(buf.Len())
	for _, r := range buf.String() {
		if r < utf8.RuneSelf {
			b.WriteRune(r)
		} else {
			fmt.Fprintf(&b, "&#%d;", r)
		}
	}
	return escapedXMLText{b.String()}
}
