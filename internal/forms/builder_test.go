package forms

import (
	"bufio"
	"bytes"
	"testing"
	"time"

	"github.com/la5nta/pat/cfg"
)

func TestInsertionTagReplacer(t *testing.T) {
	m := &Manager{config: Config{
		MyCall:     "LA5NTA",
		AppVersion: "Pat v1.0.0 (test)",
		GPSd:       cfg.GPSdConfig{Addr: gpsMockAddr},
	}}
	location = time.FixedZone("UTC+1", 1*60*60)
	now = func() time.Time { return time.Date(1988, 3, 21, 00, 00, 00, 00, location).In(time.UTC) }
	tests := map[string]string{
		"<ProgramVersion>": "Pat v1.0.0 (test)",
		"<Callsign>":       "LA5NTA",
		"<MsgSender>":      "LA5NTA",

		"<DateTime>":  "1988-03-21 00:00:00",
		"<UDateTime>": "1988-03-20 23:00:00Z",
		"<Date>":      "1988-03-21",
		"<UDate>":     "1988-03-20Z",
		"<UDTG>":      "202300Z MAR 1988",
		"<Time>":      "00:00:00",
		"<UTime>":     "23:00:00Z",
		"<Day>":       "Monday",
		"<UDay>":      "Sunday",

		"<GPS>":                "59-24.83N 005-16.08E",
		"<GPS_DECIMAL>":        "59.4138N 5.2680E",
		"<GPS_SIGNED_DECIMAL>": "59.4138 5.2680",
		"<GridSquare>":         "JO29PJ",
		"<Latitude>":           "59.4138",
		"<Longitude>":          "5.2680",
		"<GPSValid>":           "YES",
		"<GPSLatitude>":        "59.4138",
		"<GPSLongitude>":       "5.2680",
	}
	for in, expect := range tests {
		t.Run(in, func(t *testing.T) {
			if out := insertionTagReplacer(m, "<", ">")(in); out != expect {
				t.Errorf("Expected %q, got %q", expect, out)
			}
		})
	}
}

func TestBuildXML(t *testing.T) {
	location = time.FixedZone("UTC+1", 1*60*60)
	now = func() time.Time { return time.Date(1988, 3, 21, 00, 00, 00, 00, location).In(time.UTC) }
	b := messageBuilder{
		FormsMgr: &Manager{config: Config{
			MyCall:     "LA5NTA",
			AppVersion: "v1.0.0",
			Locator:    "JO29PJ",
			GPSd:       cfg.GPSdConfig{Addr: gpsMockAddr},
		}},
		Template: Template{DisplayFormPath: "viewer.html", ReplyTemplatePath: "reply.txt"},
		FormValues: map[string]string{
			"var1": "foo",
			"var2": "bar",
			"var3": "  baz \t\n", // Leading and trailing whitespace trimmed
		},
	}
	expect := []byte(`
        <?xml version="1.0" encoding="UTF-8"?>
        <RMS_Express_Form>
          <form_parameters>
            <xml_file_version>1.0</xml_file_version>
            <rms_express_version>v1.0.0</rms_express_version>
            <submission_datetime>19880320230000</submission_datetime>
            <senders_callsign>LA5NTA</senders_callsign>
            <grid_square>JO29PJ</grid_square>
            <display_form>viewer.html</display_form>
            <reply_template>reply.txt</reply_template>
          </form_parameters>
          <variables>
            <var1>foo</var1>
            <var2>bar</var2>
            <var3>baz</var3>
          </variables>
        </RMS_Express_Form>
	`)
	if got := b.buildXML(); !xmlEqual(t, got, expect) {
		t.Errorf("Got unexpected XML:\n%s\n", string(got))
	}
}

// xmlEqual compares two byte slices line by line, ignoring leading/trailing whitespace.
func xmlEqual(t *testing.T, a, b []byte) bool {
	lines := func(xml []byte) (slice [][]byte) {
		s := bufio.NewScanner(bytes.NewReader(xml))
		for s.Scan() {
			l := bytes.TrimSpace(s.Bytes())
			if len(l) > 0 {
				slice = append(slice, l)
			}
		}
		return slice
	}
	aLines, bLines := lines(a), lines(b)
	for i := 0; i < len(aLines) && i < len(bLines); i++ {
		if !bytes.Equal(aLines[i], bLines[i]) {
			t.Errorf("%q != %q", aLines[i], bLines[i])
			return false
		}
	}
	return len(aLines) == len(bLines)
}
