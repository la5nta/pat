package forms

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/la5nta/pat/cfg"
)

// FuzzEscapeXMLText checks the escaping contract for arbitrary text: the
// output is pure ASCII, and the standard XML decoder resolves it back to the
// original runes — except runes unrepresentable in XML 1.0, which the
// contract replaces with U+FFFD.
func FuzzEscapeXMLText(f *testing.F) {
	f.Add("plain")
	f.Add("Café")
	f.Add("日本語")
	f.Add(`&<>"'`)
	f.Add("a\tb\nc\rd")
	f.Add("\x00\x01\x7f")
	f.Add("\xff\xfe")
	f.Fuzz(func(t *testing.T, s string) {
		escaped := escapeXMLText(s).Value

		for i, c := range []byte(escaped) {
			if c > unicode.MaxASCII {
				t.Fatalf("Non-ASCII byte %#x at offset %d: %q", c, i, escaped)
			}
		}

		var doc struct {
			Value string `xml:",chardata"`
		}
		if err := xml.Unmarshal([]byte("<a>"+escaped+"</a>"), &doc); err != nil {
			t.Fatalf("Escaped text is not valid XML: %v: %q", err, escaped)
		}
		if want := xmlDecodedValue(s); doc.Value != want {
			t.Fatalf("Decoded %q, want %q (input %q)", doc.Value, want, s)
		}
	})
}

// FuzzBuildXML checks the full form document for arbitrary variable names and
// values: the document must be well-formed and pure ASCII (Winlink Express
// does not honor the encoding declaration), and the standard XML decoder must
// resolve it back to the same form parameters and variables.
func FuzzBuildXML(f *testing.F) {
	f.Add("var1", "Café", "var2", "日本語")
	f.Add("télé", "accents & <stuff>", "var1", `'"`)
	f.Add("var1", "\x00\xff\x7f", "var2", "")
	f.Add("same", "first", "same", "second")
	f.Add("a1", "a\tb\nc\rd", "a2", "&amp; &#233; literal")
	f.Add("variables", "nested", "var2", "x")
	f.Fuzz(func(t *testing.T, name1, value1, name2, value2 string) {
		// Variable names are written verbatim — character references are
		// not allowed in XML names, see writeTag — so the contract only
		// holds for names that are valid XML element names. Anything else
		// is a malformed form template.
		if !isXMLName(name1) || !isXMLName(name2) {
			t.Skip()
		}
		b := messageBuilder{
			FormsMgr: &Manager{config: Config{
				MyCall:          "LA5NTA",
				AppVersion:      "v1.0.0",
				GPSd:            cfg.GPSdConfig{Addr: gpsMockAddr},
				LocatorProvider: mockLocatorProvider("JO29PJ"),
			}},
			Template: Template{
				DisplayFormPath:   "viewer.html",
				ReplyTemplatePath: "Accusé de réception.txt",
			},
			FormValues: map[string]string{name1: value1, name2: value2},
		}

		got := b.buildXML()

		// The document must be pure ASCII, so that Winlink Express — which
		// does not honor the encoding declaration — renders every value.
		// Non-ASCII names are the exception: they are written verbatim,
		// since references are not allowed in names.
		if isASCII(name1) && isASCII(name2) {
			for i, c := range got {
				if c > unicode.MaxASCII {
					t.Fatalf("Non-ASCII byte %#x at offset %d:\n%s", c, i, got)
				}
			}
		}

		var back struct {
			XMLName        xml.Name
			FormParameters struct {
				Nodes []struct {
					XMLName xml.Name
					Value   string `xml:",chardata"`
				} `xml:",any"`
			} `xml:"form_parameters"`
			Variables struct {
				Nodes []struct {
					XMLName xml.Name
					Value   string `xml:",chardata"`
				} `xml:",any"`
			} `xml:"variables"`
		}
		if err := xml.Unmarshal(got, &back); err != nil {
			t.Fatalf("Output is not valid XML: %v:\n%s", err, got)
		}
		if back.XMLName.Local != "RMS_Express_Form" {
			t.Fatalf("Root element = %q, want RMS_Express_Form:\n%s", back.XMLName.Local, got)
		}

		// The static fields must survive verbatim, with character
		// references resolved.
		params := make(map[string]string)
		for _, n := range back.FormParameters.Nodes {
			params[n.XMLName.Local] = n.Value
		}
		for k, want := range map[string]string{
			"xml_file_version": "1.0",
			"senders_callsign": "LA5NTA",
			"display_form":     "viewer.html",
			"reply_template":   "Accusé de réception.txt",
		} {
			if have := params[k]; have != want {
				t.Fatalf("Form parameter %q: got %q, want %q:\n%s", k, have, want, got)
			}
		}

		// The variables must decode back to the same map, with values as
		// sent (values are trimmed and escaped; map literal semantics give
		// the later value precedence for duplicate keys).
		wantVars := map[string]string{
			name1: xmlDecodedValue(strings.TrimSpace(value1)),
			name2: xmlDecodedValue(strings.TrimSpace(value2)),
		}
		gotVars := make(map[string]string)
		for _, n := range back.Variables.Nodes {
			if _, dup := gotVars[n.XMLName.Local]; dup {
				t.Fatalf("Duplicate variable %q:\n%s", n.XMLName.Local, got)
			}
			gotVars[n.XMLName.Local] = n.Value
		}
		if len(gotVars) != len(wantVars) {
			t.Fatalf("Got %d variables, want %d:\n%s", len(gotVars), len(wantVars), got)
		}
		for name, want := range wantVars {
			if gotVars[name] != want {
				t.Fatalf("Variable %q: got %q, want %q:\n%s", name, gotVars[name], want, got)
			}
		}
	})
}

// isInXMLCharRange reports whether r is in the XML 1.0 Char production, i.e.
// representable in character data or as a character reference. It mirrors
// the rule xml.EscapeText applies when deciding whether to replace a rune
// with U+FFFD. Of the control characters, only \t, \n and \r are included;
// they are handled in xmlDecodedValue.
func isInXMLCharRange(r rune) bool {
	return 0x20 <= r && r <= 0xD7FF ||
		0xE000 <= r && r <= 0xFFFD ||
		0x10000 <= r && r <= 0x10FFFF
}

// xmlDecodedValue returns s as it must come back out of the standard XML
// decoder, given the escaping contract of escapeXMLText: every rune is
// preserved, except runes unrepresentable in XML 1.0, which are replaced with
// U+FFFD. Invalid UTF-8 sequences yield U+FFFD when ranging over the string,
// and U+FFFD itself is representable, so both come back as U+FFFD.
func xmlDecodedValue(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			b.WriteRune(r)
		case isInXMLCharRange(r):
			b.WriteRune(r)
		default:
			b.WriteRune(utf8.RuneError)
		}
	}
	return b.String()
}

// isXMLName reports whether s is a valid XML element name, by asking the
// standard XML decoder itself. Names containing a colon (a namespace prefix)
// are rejected, since the decoder splits them into Space and Local.
func isXMLName(s string) bool {
	if s == "" {
		return false
	}
	dec := xml.NewDecoder(strings.NewReader("<" + s + "/>"))
	tok, err := dec.Token()
	if err != nil {
		return false
	}
	start, ok := tok.(xml.StartElement)
	if !ok || start.Name.Local != s {
		return false
	}
	if _, err := dec.Token(); err != nil {
		return false // not a single self-closing element
	}
	_, err = dec.Token()
	return err == io.EOF
}

// isASCII reports whether s contains only ASCII characters.
func isASCII(s string) bool {
	for _, r := range s {
		if r >= utf8.RuneSelf {
			return false
		}
	}
	return true
}
