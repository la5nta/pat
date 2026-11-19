package forms

import (
	"bytes"
	"encoding/xml"
	"testing"
	"unicode"
)

func TestEscapeXMLText(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"plain 123", "plain 123"},
		{`&<>"'`, "&amp;&lt;&gt;&#34;&#39;"},
		{"a\tb\nc\rd", "a&#x9;b&#xA;c&#xD;d"},
		{"Café", "Caf&#233;"},
		{"日本語", "&#26085;&#26412;&#35486;"},
		// DEL is ASCII and representable, and passes through.
		{"a\x7fb", "a\x7fb"},
		// Unrepresentable control characters are replaced by U+FFFD,
		// written as a reference to keep the document pure ASCII.
		{"a\x00b\x0cc", "a&#65533;b&#65533;c"},
		// Invalid UTF-8 is replaced by U+FFFD when ranging over the string.
		{"a\xffb", "a&#65533;b"},
	}
	for _, tt := range tests {
		if got := escapeXMLText(tt.in).Value; got != tt.want {
			t.Errorf("escapeXMLText(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// marshalFormXML marshals a form document the same way as
// messageBuilder.buildXML: static params with pre-escaped filename elements,
// dynamic variables with pre-escaped values.
func marshalFormXML(t *testing.T, params formParams, variables []formVariable) []byte {
	t.Helper()
	form := struct {
		XMLName   xml.Name       `xml:"RMS_Express_Form"`
		Params    formParams     `xml:"form_parameters"`
		Variables []formVariable `xml:"variables>name"`
	}{
		XMLName:   xml.Name{Local: "RMS_Express_Form"},
		Params:    params,
		Variables: variables,
	}
	data, err := xml.MarshalIndent(form, "", "    ")
	if err != nil {
		t.Fatalf("Failed to marshal form XML: %v", err)
	}
	return append([]byte(xml.Header), data...)
}

func TestFormXMLBytes(t *testing.T) {
	got := marshalFormXML(t, formParams{
		XMLFileVersion:     "1.0",
		RMSExpressVersion:  "v1.0.0",
		SubmissionDatetime: "19880320230000",
		SendersCallsign:    "LA5NTA",
		GridSquare:         "JO29PJ",
		DisplayForm:        escapeXMLText("viewer.html"),
		ReplyTemplate:      escapeXMLText("Accusé de réception.txt"),
	}, []formVariable{
		{xml.Name{Local: "var1"}, escapeXMLText("éèçàù")},
	})

	// The document must be pure ASCII, so that Winlink Express — which does
	// not honor the encoding declaration — renders every character.
	for i, c := range got {
		if c > unicode.MaxASCII {
			t.Fatalf("Non-ASCII byte %#x at offset %d:\n%s", c, i, got)
		}
	}

	// It must also be byte-exact, pinning the document shape against
	// encoding/xml's marshalling of the same content.
	want := xml.Header + `<RMS_Express_Form>
    <form_parameters>
        <xml_file_version>1.0</xml_file_version>
        <rms_express_version>v1.0.0</rms_express_version>
        <submission_datetime>19880320230000</submission_datetime>
        <senders_callsign>LA5NTA</senders_callsign>
        <grid_square>JO29PJ</grid_square>
        <display_form>viewer.html</display_form>
        <reply_template>Accus&#233; de r&#233;ception.txt</reply_template>
    </form_parameters>
    <variables>
        <var1>&#233;&#232;&#231;&#224;&#249;</var1>
    </variables>
</RMS_Express_Form>`
	if !bytes.Equal(got, []byte(want)) {
		t.Errorf("Got:\n%s\nWant:\n%s", got, want)
	}
}

func TestFormXMLBytesEmptyVariables(t *testing.T) {
	// An empty variable list is serialized as an empty variables element,
	// matching encoding/xml's marshalling of the same content.
	got := marshalFormXML(t, formParams{
		DisplayForm:   escapeXMLText("viewer.html"),
		ReplyTemplate: escapeXMLText("reply.txt"),
	}, nil)
	if !bytes.Contains(got, []byte("<variables></variables>")) {
		t.Errorf("Missing empty variables element in:\n%s", got)
	}
}

func TestFormXMLRoundTrip(t *testing.T) {
	// The output must parse back to the same content, with character
	// references resolved, proving it is valid and semantically unchanged
	// for conformant readers — including the hand-escaped filename
	// elements.
	got := marshalFormXML(t, formParams{
		XMLFileVersion:     "1.0",
		RMSExpressVersion:  "v1.0.0",
		SubmissionDatetime: "19880320230000",
		SendersCallsign:    "LA5NTA",
		GridSquare:         "JO29PJ",
		DisplayForm:        escapeXMLText("viewer.html"),
		ReplyTemplate:      escapeXMLText("Accusé de réception.txt"),
	}, []formVariable{
		{xml.Name{Local: "var1"}, escapeXMLText(`Café & "thé"`)},
		{xml.Name{Local: "var2"}, escapeXMLText("")},
	})

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
		t.Errorf("Root element = %q, want RMS_Express_Form", back.XMLName.Local)
	}
	params := make(map[string]string)
	for _, n := range back.FormParameters.Nodes {
		params[n.XMLName.Local] = n.Value
	}
	wantParams := map[string]string{
		"xml_file_version":    "1.0",
		"rms_express_version": "v1.0.0",
		"submission_datetime": "19880320230000",
		"senders_callsign":    "LA5NTA",
		"grid_square":         "JO29PJ",
		"display_form":        "viewer.html",
		"reply_template":      "Accusé de réception.txt",
	}
	for k, want := range wantParams {
		if got := params[k]; got != want {
			t.Errorf("form parameter %q: got %q, want %q", k, got, want)
		}
	}
	wantVars := []struct{ name, value string }{
		{"var1", `Café & "thé"`},
		{"var2", ""},
	}
	if len(back.Variables.Nodes) != len(wantVars) {
		t.Fatalf("Got %d variables, want %d:\n%s", len(back.Variables.Nodes), len(wantVars), got)
	}
	for i, want := range wantVars {
		n := back.Variables.Nodes[i]
		if n.XMLName.Local != want.name || n.Value != want.value {
			t.Errorf("Variable %d: got %s=%q, want %s=%q", i, n.XMLName.Local, n.Value, want.name, want.value)
		}
	}
}
