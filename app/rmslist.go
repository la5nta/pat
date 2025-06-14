// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/la5nta/pat/internal/cmsapi"
	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/pat/internal/directories"

	"github.com/pd0mz/go-maidenhead"
)

type JSONURL struct{ url.URL }

func (url JSONURL) MarshalJSON() ([]byte, error) { return json.Marshal(url.String()) }

// JSONFloat64 is a float64 which serializes NaN and Inf(+-) as JSON value null
type JSONFloat64 float64

func (f JSONFloat64) MarshalJSON() ([]byte, error) {
	if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
		return json.Marshal(nil)
	}
	return json.Marshal(float64(f))
}

type RMS struct {
	Callsign   string      `json:"callsign"`
	Gridsquare string      `json:"gridsquare"`
	Distance   JSONFloat64 `json:"distance"`
	Azimuth    JSONFloat64 `json:"azimuth"`
	Modes      string      `json:"modes"`
	Freq       Frequency   `json:"freq"`
	Dial       Frequency   `json:"dial"`
	URL        *JSONURL    `json:"url"`
}

func (r RMS) IsMode(mode string) bool {
	if mode == MethodVaraFM {
		return strings.HasPrefix(r.Modes, "VARA FM")
	}
	if mode == MethodVaraHF {
		return strings.HasPrefix(r.Modes, "VARA") && !strings.HasPrefix(r.Modes, "VARA FM")
	}
	return strings.Contains(strings.ToLower(r.Modes), mode)
}

func (r RMS) IsBand(band string) bool {
	return bands[band].Contains(r.Freq)
}

type ByDist []RMS

func (r ByDist) Len() int           { return len(r) }
func (r ByDist) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r ByDist) Less(i, j int) bool { return r[i].Distance < r[j].Distance }

func (a *App) ReadRMSList(ctx context.Context, forceDownload bool, filterFn func(rms RMS) (keep bool)) ([]RMS, error) {
	me, err := maidenhead.ParseLocator(a.config.Locator)
	if err != nil {
		log.Print("Missing or Invalid Locator, will not compute distance and Azimuth")
	}

	fileName := "rmslist"
	isDefaultServiceCode := len(a.config.ServiceCodes) == 1 && a.config.ServiceCodes[0] == "PUBLIC"
	if !isDefaultServiceCode {
		fileName += "-" + strings.Join(a.config.ServiceCodes, "-")
	}
	filePath := filepath.Join(directories.DataDir(), fileName+".json")
	debug.Printf("RMS list file is %s", filePath)

	f, err := cmsapi.GetGatewayStatusCached(ctx, filePath, forceDownload, a.config.ServiceCodes...)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var status cmsapi.GatewayStatus
	if err = json.NewDecoder(f).Decode(&status); err != nil {
		return nil, err
	}

	slice := []RMS{}
	for _, gw := range status.Gateways {
		for _, channel := range gw.Channels {
			r := RMS{
				Callsign:   gw.Callsign,
				Gridsquare: channel.Gridsquare,
				Modes:      channel.SupportedModes,
				Freq:       Frequency(channel.Frequency),
				Dial:       Frequency(channel.Frequency).Dial(channel.SupportedModes),
			}
			if chURL := toURL(channel, gw.Callsign); chURL != nil {
				r.URL = &JSONURL{*chURL}
			}
			hasLocator := me != maidenhead.Point{}
			if them, err := maidenhead.ParseLocator(channel.Gridsquare); err == nil && hasLocator {
				r.Distance = JSONFloat64(me.Distance(them))
				r.Azimuth = JSONFloat64(me.Bearing(them))
			}
			if keep := filterFn(r); !keep {
				continue
			}
			slice = append(slice, r)
		}
	}
	return slice, nil
}

func toURL(gc cmsapi.GatewayChannel, targetCall string) *url.URL {
	freq := Frequency(gc.Frequency).Dial(gc.SupportedModes)
	chURL, _ := url.Parse(fmt.Sprintf("%s:///%s?freq=%v", toTransport(gc), targetCall, freq.KHz()))
	addBandwidth(gc, chURL)
	return chURL
}

func addBandwidth(gc cmsapi.GatewayChannel, chURL *url.URL) {
	bw := ""
	modeF := strings.Fields(gc.SupportedModes)
	switch modeF[0] {
	case "ARDOP":
		if len(modeF) > 1 {
			bw = modeF[1] + "MAX"
		}
	case "VARA":
		if len(modeF) > 1 && modeF[1] == "FM" {
			// VARA FM should not set bandwidth in connect URL or sent over the command port,
			// it's set in the VARA Setup dialog
			bw = ""
		} else {
			// VARA HF may be 500, 2750, or none which is implicitly 2300
			if len(modeF) > 1 {
				if len(modeF) > 1 {
					bw = modeF[1]
				}
			} else {
				bw = "2300"
			}
		}
	}
	if bw != "" {
		v := chURL.Query()
		v.Set("bw", bw)
		chURL.RawQuery = v.Encode()
	}
}

var transports = []string{MethodAX25, MethodPactor, MethodArdop, MethodVaraFM, MethodVaraHF}

func toTransport(gc cmsapi.GatewayChannel) string {
	modes := strings.ToLower(gc.SupportedModes)
	for _, transport := range transports {
		if strings.Contains(modes, "packet") {
			// bug(maritnhpedersen): We really don't know which transport to use here. It could be serial-tnc or ax25, but ax25 is most likely.
			return MethodAX25
		}
		if strings.HasPrefix(modes, "vara fm") {
			return MethodVaraFM
		}
		if strings.HasPrefix(modes, "vara") {
			return MethodVaraHF
		}
		if strings.Contains(modes, transport) {
			return transport
		}
	}
	return ""
}
