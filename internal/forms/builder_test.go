package forms

import (
	"testing"
	"time"

	"github.com/la5nta/pat/cfg"
)

func TestInsertionTagReplacer(t *testing.T) {
	m := &Manager{config: Config{
		MyCall:     "LA5NTA",
		AppVersion: "v1.0.0",
		GPSd:       cfg.GPSdConfig{Addr: gpsMockAddr},
	}}
	location = time.FixedZone("UTC+1", 1*60*60)
	now = func() time.Time { return time.Date(1988, 3, 21, 00, 00, 00, 00, location).In(time.UTC) }
	tests := map[string]string{
		"<ProgramVersion>": "Pat v1.0.0",
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
		"<GPSValid>":           "YES ", // This trailing space appears to be intentional,
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
