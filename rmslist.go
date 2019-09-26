// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"log"
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

type rms struct {
	callsign string
	gridsq   string
	distance float64
	azimuth  float64
	modes    string
	freq     string
	dial     string
	url      *url.URL
}

type byDist []rms

func (r byDist) Len() int           { return len(r) }
func (r byDist) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r byDist) Less(i, j int) bool { return r[i].distance < r[j].distance }

func rmsListHandle(args []string) {
	set := pflag.NewFlagSet("rmslist", pflag.ExitOnError)
	mode := set.StringP("mode", "m", "", "")
	band := set.StringP("band", "b", "", "")
	forceDownload := set.BoolP("force-download", "d", false, "")
	byDistance := set.BoolP("sort-distance", "s", false, "")
	set.Parse(args)

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

	var query string
	if len(set.Args()) > 0 {
		query = strings.ToUpper(set.Args()[0])
	}

	file, err := cmsapi.GetGatewayStatusCached(filePath, *forceDownload, config.ServiceCodes...)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	var status cmsapi.GatewayStatus

	err = json.NewDecoder(file).Decode(&status)
	if err != nil {
		log.Fatal(err)
	}

	noLocator := false

	me, err := maidenhead.ParseLocator(config.Locator)
	if err != nil {
		log.Print("Missing or Invalid Locator, will not compute distance and Azimuth")
		noLocator = true
	}

	*mode = strings.ToLower(*mode)

	rList := []rms{}

	// Print gateways (separated by blank line)
	for _, gw := range status.Gateways {
		switch {
		case query != "" && !strings.HasPrefix(gw.Callsign, query):
			continue
		default:
		}

		for _, channel := range gw.Channels {

			r := rms{
				callsign: gw.Callsign,
				gridsq:   channel.Gridsquare,
				modes:    channel.SupportedModes,
			}

			f := Frequency(channel.Frequency)
			r.dial = f.Dial(channel.SupportedModes).String()
			r.freq = f.String()

			switch {
			case mode != nil && !strings.Contains(strings.ToLower(channel.SupportedModes), *mode):
				continue
			case !bands[*band].Contains(f):
				continue
			}
			r.distance = float64(0)
			r.azimuth = float64(0)
			if !noLocator {
				if them, err := maidenhead.ParseLocator(channel.Gridsquare); err == nil {
					r.distance = me.Distance(them)
					r.azimuth = me.Bearing(them)
				}
			}

			r.url = toURL(channel, gw.Callsign)

			rList = append(rList, r)
		}
	}
	if *byDistance {
		sort.Sort(byDist(rList))
	}
	fmtStr := "%-9.9s [%-6.6s] %-6.6s %3.3s %-15.15s %14.14s %14.14s %s\n"

	// Print header
	fmt.Printf(fmtStr, "callsign", "gridsq", "dist", "Az", "mode(s)", "dial freq", "center freq", "url") //TODO: "center frequency" of packet is wrong...

	for i := 0; i < len(rList); i++ {
		r := rList[i]
		distance := strconv.FormatFloat(r.distance, 'f', 0, 64)
		azimuth := strconv.FormatFloat(r.azimuth, 'f', 0, 64)

		fmt.Printf(fmtStr, r.callsign, r.gridsq, distance, azimuth, r.modes, r.dial, r.freq, r.url)
		if i+1 < len(rList) && rList[i].callsign != rList[i+1].callsign {
			fmt.Println("")
		}
	}
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
