package gpsd

import (
	"encoding/json"
	"errors"
	"time"
)

// A Sky object reports a sky view of the GPS satellite positions.
type Sky struct {
	Device string    `json:"device,omitempty"`
	Time   time.Time `json:"time,omitempty"`

	XDOP, YDOP, VDOP, TDOP, HDOP, PDOP, GDOP json.Number

	Satellites []Satellite `json:"satellites"`
}

// A TPV object is a time-position-velocity report.
type TPV struct {
	Device string      // Name of originating device.
	Mode   NMEAMode    // NMEA mode: %d, 0=no mode value yet seen, 1=no fix, 2=2D, 3=3D.
	Time   time.Time   // Time/date stamp.  May have a fractional part of up to .001sec precision. May be absent if mode is not 2D or 3D.
	EPT    json.Number // Estimated timestamp error (%f, seconds, 95% confidence). Present if time is present.

	Lat, Lon, Alt       json.Number
	EPX, EPY, EPV       json.Number // Lat, Lon, Alt error estimate in meters, 95% confidence. Present if mode is 2 or 3 and DOPs can be calculated from the satellite view.
	Track, Speed, Climb json.Number
	EPD, EPS, EPC       json.Number
}

func (t TPV) Position() Position {
	lat, _ := t.Lat.Float64()
	lon, _ := t.Lon.Float64()
	alt, _ := t.Alt.Float64()
	track, _ := t.Track.Float64()
	speed, _ := t.Speed.Float64()

	return Position{Lat: lat, Lon: lon, Alt: alt, Track: track, Speed: speed, Time: t.Time}
}

func (t TPV) HasFix() bool { return t.Mode > ModeNoFix }

// Satellite represents a GPS satellite.
type Satellite struct {
	// PRN ID of the satellite. 1-63 are GNSS satellites, 64-96 are GLONASS satellites, 100-164 are SBAS satellites.
	PRN int `json:"PRN"`

	// Azimuth, degrees from true north.
	Azimuth json.Number `json:"az"`

	// Elevation in degrees.
	Elevation json.Number `json:"el"`

	// Signal strength in dB.
	SignalStrength json.Number `json:"ss"`

	// Used in current solution?
	//
	// (SBAS/WAAS/EGNOS satellites may be flagged used if the solution has corrections from them, but not all drivers make this information available).
	Used bool `json:"used"`
}

// Version holds GPSd version data.
type Version struct {
	Release    string `json:"release"`
	Rev        string `json:"rev"`
	ProtoMajor int    `json:"proto_major"`
	ProtoMinor int    `json:"proto_minor"`
}

// Device represents a connected sensor/GPS.
type Device struct {
	Path     string `json:"path,omitempty"`
	Flags    *int   `json:"flags,omitempty"`
	Driver   string `json:"driver,omitempty"`
	Subtype  string `json:"subtype,omitempty"`
	Bps      *int   `json:"bps,omitempty"`
	Parity   string `json:"parity"`
	StopBits int    `json:"stopbits"`

	// Activated time.Time `json:"activated,omitempty"` (Must parse as fractional epoch time)
}

type watch struct {
	Class   string `json:"class"`
	Enable  bool   `json:"enable,omitempty"`
	JSON    *bool  `json:"json,omitempty"`
	NMEA    *bool  `json:"nmea,omitempty"`
	Raw     *int   `json:"raw,omitempty"`
	Scaled  *bool  `json:"scaled,omitempty"`
	Split24 *bool  `json:"split24,omitempty"`
	PPS     *bool  `json:"pps,omitempty"`
	Device  string `json:"device,omitempty"`

	Devices []Device `json:"devices,omitempty"` // Only in response
}

func parseJSONObject(raw []byte) (interface{}, error) {
	var class struct{ Class string }
	err := json.Unmarshal(raw, &class)
	if err != nil {
		return nil, err
	}

	switch class.Class {
	case "WATCH":
		var w watch
		err = json.Unmarshal(raw, &w)
		return w, err
	case "DEVICES":
		var devs struct{ Devices []Device }
		err = json.Unmarshal(raw, &devs)
		return devs.Devices, err
	case "DEVICE":
		var dev Device
		err = json.Unmarshal(raw, &dev)
		return dev, err
	case "VERSION":
		var ver Version
		err = json.Unmarshal(raw, &ver)
		return ver, err
	case "ERROR":
		var err struct{ Message string }
		json.Unmarshal(raw, &err)
		return nil, errors.New(err.Message)
	case "SKY":
		var sky Sky
		err = json.Unmarshal(raw, &sky)
		return sky, err
	case "TPV":
		var tpv TPV
		err = json.Unmarshal(raw, &tpv)
		return tpv, err
	default:
		var m map[string]interface{}
		err = json.Unmarshal(raw, &m)
		return m, err
	}
}
