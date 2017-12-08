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

	"github.com/la5nta/pat/internal/cmsapi"
)

func rmsListHandle(args []string) {
	set := pflag.NewFlagSet("rmslist", pflag.ExitOnError)
	mode := set.StringP("mode", "m", "", "")
	band := set.StringP("band", "b", "", "")
	forceDownload := set.BoolP("force-download", "d", false, "")
	set.Parse(args)

	appDir, err := mailbox.DefaultAppDir()
	if err != nil {
		log.Fatal(err)
	}
	filePath := path.Join(appDir, "rmslist.json") // Should be moved to a tmp-folder, along with logfile.

	var query string
	if len(set.Args()) > 0 {
		query = strings.ToUpper(set.Args()[0])
	}

	file, err := cmsapi.GetGatewayStatusCached(filePath, *forceDownload)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	var status cmsapi.GatewayStatus

	err = json.NewDecoder(file).Decode(&status)
	if err != nil {
		log.Fatal(err)
	}

	*mode = strings.ToLower(*mode)

	fmtStr := "%-9.9s [%-6.6s] %-15.15s %14.14s %14.14s %s\n"

	// Print header
	fmt.Printf(fmtStr, "callsign", "gridsq", "mode(s)", "dial freq", "center freq", "url") //TODO: "center frequency" of packet is wrong...

	// Print gateways (separated by blank line)
	for _, gw := range status.Gateways {
		switch {
		case query != "" && !strings.HasPrefix(gw.Callsign, query):
			continue
		default:
		}

		var printed bool
		for _, channel := range gw.Channels {
			freq := Frequency(channel.Frequency)
			dial := freq.Dial(channel.SupportedModes)

			switch {
			case mode != nil && !strings.Contains(strings.ToLower(channel.SupportedModes), *mode):
				continue
			case !bands[*band].Contains(freq):
				continue
			default:
				printed = true
			}

			url := toURL(channel, gw.Callsign)
			fmt.Printf(fmtStr, gw.Callsign, channel.Gridsquare, channel.SupportedModes, dial, freq, url)
		}
		if printed {
			fmt.Println("")
		}
	}
}

func toURL(gc cmsapi.GatewayChannel, targetcall string) *url.URL {
	freq := Frequency(gc.Frequency).Dial(gc.SupportedModes)

	url, _ := url.Parse(fmt.Sprintf("%s:///%s?freq=%v", toTransport(gc), targetcall, freq.KHz()))
	return url
}

func toTransport(gc cmsapi.GatewayChannel) string {
	modes := strings.ToLower(gc.SupportedModes)
	switch {
	case strings.Contains(modes, "winmor"):
		return "winmor"
	case strings.Contains(modes, "packet"):
		return "ax25"
	case strings.Contains(modes, "pactor"):
		return "pactor"
	case strings.Contains(modes, "ardop"):
		return "ardop"
	default:
		return ""
	}
}
