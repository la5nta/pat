// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/url"
	"path"
	"strings"

	"github.com/spf13/pflag"

	"github.com/la5nta/wl2k-go/mailbox"

	"sort"
	"strconv"

	"github.com/la5nta/pat/internal/cmsapi"
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
	return strings.Contains(strings.ToLower(r.Modes), mode)
}

func (r RMS) IsBand(band string) bool {
	return bands[band].Contains(r.Freq)
}

type byDist []RMS

func (r byDist) Len() int           { return len(r) }
func (r byDist) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r byDist) Less(i, j int) bool { return r[i].Distance < r[j].Distance }

func rmsListHandle(args []string) {
	set := pflag.NewFlagSet("rmslist", pflag.ExitOnError)
	mode := set.StringP("mode", "m", "", "")
	band := set.StringP("band", "b", "", "")
	forceDownload := set.BoolP("force-download", "d", false, "")
	byDistance := set.BoolP("sort-distance", "s", false, "")
	set.Parse(args)

	var query string
	if len(set.Args()) > 0 {
		query = strings.ToUpper(set.Args()[0])
	}

	*mode = strings.ToLower(*mode)
	rList, err := ReadRMSList(*forceDownload, func(rms RMS) bool {
		switch {
		case query != "" && !strings.HasPrefix(rms.Callsign, query):
			return false
		case mode != nil && !rms.IsMode(*mode):
			return false
		case band != nil && !rms.IsBand(*band):
			return false
		default:
			return true
		}
	})
	if err != nil {
		log.Fatal(err)
	}
	if *byDistance {
		sort.Sort(byDist(rList))
	}

	fmtStr := "%-9.9s [%-6.6s] %-6.6s %3.3s %-15.15s %14.14s %14.14s %s\n"

	// Print header
	fmt.Printf(fmtStr, "callsign", "gridsq", "dist", "Az", "mode(s)", "dial freq", "center freq", "url")

	// Print gateways (separated by blank line)
	for i := 0; i < len(rList); i++ {
		r := rList[i]
		distance := strconv.FormatFloat(float64(r.Distance), 'f', 0, 64)
		azimuth := strconv.FormatFloat(float64(r.Azimuth), 'f', 0, 64)

		fmt.Printf(fmtStr, r.Callsign, r.Gridsquare, distance, azimuth, r.Modes, r.Dial, r.Freq, r.URL)
		if i+1 < len(rList) && rList[i].Callsign != rList[i+1].Callsign {
			fmt.Println("")
		}
	}
}

func ReadRMSList(forceDownload bool, filterFn func(rms RMS) (keep bool)) ([]RMS, error) {
	me, err := maidenhead.ParseLocator(config.Locator)
	if err != nil {
		log.Print("Missing or Invalid Locator, will not compute distance and Azimuth")
	}

	appDir, err := mailbox.DefaultAppDir()
	if err != nil {
		log.Fatal(err)
	}
	fileName := "rmslist"
	isDefaultServiceCode := len(config.ServiceCodes) == 1 && config.ServiceCodes[0] == "PUBLIC"
	if !isDefaultServiceCode {
		fileName += "-" + strings.Join(config.ServiceCodes, "-")
	}
	filePath := path.Join(appDir, fileName+".json") // Should be moved to a tmp-folder, along with logfile.

	f, err := cmsapi.GetGatewayStatusCached(filePath, forceDownload, config.ServiceCodes...)
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
			if url := toURL(channel, gw.Callsign); url != nil {
				r.URL = &JSONURL{*url}
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

func toURL(gc cmsapi.GatewayChannel, targetcall string) *url.URL {
	freq := Frequency(gc.Frequency).Dial(gc.SupportedModes)
	url, _ := url.Parse(fmt.Sprintf("%s:///%s?freq=%v", toTransport(gc), targetcall, freq.KHz()))
	return url
}

var transports = []string{"winmor", "packet", "pactor", "ardop"}

func toTransport(gc cmsapi.GatewayChannel) string {
	modes := strings.ToLower(gc.SupportedModes)
	for _, transport := range transports {
		if strings.Contains(modes, transport) {
			return transport
		}
	}
	return ""
}
