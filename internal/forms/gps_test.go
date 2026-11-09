package forms

import (
	"testing"

	"github.com/la5nta/pat/cfg"
)

func TestGPSPosStoredFallback(t *testing.T) {
	stored := &cfg.StationPosition{Lat: 59.4, Lon: 5.2}
	tests := map[string]struct {
		gpsd    cfg.GPSdConfig
		wantErr bool
	}{
		"no gpsd, allowed":       {cfg.GPSdConfig{AllowForms: true}, false},
		"gpsd down, allowed":     {cfg.GPSdConfig{AllowForms: true, Addr: "127.0.0.1:1"}, false},
		"no gpsd, not allowed":   {cfg.GPSdConfig{}, true},
		"gpsd down, not allowed": {cfg.GPSdConfig{Addr: "127.0.0.1:1"}, true},
	}
	for name, tc := range tests {
		m := &Manager{config: Config{GPSd: tc.gpsd, StationPosition: stored}}
		pos, err := m.gpsPos()
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err=%v, wantErr=%v", name, err, tc.wantErr)
		} else if err == nil && (pos.Lat != 59.4 || pos.Lon != 5.2) {
			t.Errorf("%s: got %+v", name, pos)
		}
	}
	m := &Manager{config: Config{GPSd: cfg.GPSdConfig{AllowForms: true}}}
	if _, err := m.gpsPos(); err == nil {
		t.Error("expected error with no stored position and no gpsd")
	}
}
