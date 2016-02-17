// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/spf13/pflag"

	"github.com/la5nta/wl2k-go/mailbox"
)

const GatewayStatusUrl = "http://server.winlink.org:8085/gateway/status.json"

type GatewayStatus struct {
	ServerName string    `json:"ServerName"`
	ErrorCode  int       `json:"ErrorCode"`
	Gateways   []Gateway `json:"Gateways"`
}

type Gateway struct {
	Callsign      string
	BaseCallsign  string
	RequestedMode string
	Comments      string
	LastStatus    RFC1123Time
	Latitude      float64
	Longitude     float64

	Channels []GatewayChannel `json:"GatewayChannels"`
}

type GatewayChannel struct {
	OperatingHours string
	SupportedModes string
	Frequency      float64
	ServiceCode    string
	Baud           string
	RadioRange     string
	Mode           int
	Gridsquare     string
	Antenna        string
}

type RFC1123Time struct{ time.Time }

// GetGatewayStatus fetches the gateway status list returned by GatewayStatusUrl
//
// mode can be any of [packet, pactor, winmor, robustpacket, allhf or anyall]. Empty is AnyAll.
// historyHours is the number of hours of history to include (maximum: 48). If < 1, then API default is used.
// serviceCodes defaults to "PUBLIC".
func GetGatewayStatus(mode string, historyHours int, serviceCodes ...string) (io.ReadCloser, error) {
	switch {
	case mode == "":
		mode = "AnyAll"
	case historyHours > 48:
		historyHours = 48
	case len(serviceCodes) == 0:
		serviceCodes = []string{"PUBLIC"}
	}

	params := url.Values{"Mode": {mode}}
	if historyHours >= 0 {
		params.Add("HistoryHours", fmt.Sprintf("%d", historyHours))
	}
	for _, str := range serviceCodes {
		params.Add("ServiceCodes", str)
	}

	resp, err := http.PostForm(GatewayStatusUrl, params)
	switch {
	case err != nil:
		return nil, err
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("Unexpected http status '%s'.", resp.Status)
	}

	return resp.Body, err
}

func GetGatewayStatusCached(cacheFile string, forceDownload bool) (io.ReadCloser, error) {
	if !forceDownload {
		file, err := os.Open(cacheFile)
		if err == nil {
			return file, nil
		}
	}

	file, err := os.Create(cacheFile)
	if err != nil {
		return nil, err
	}

	log.Println("Downloading latest gateway status information...")
	fresh, err := GetGatewayStatus("", 48)
	if err != nil {
		return nil, err
	}

	_, err = io.Copy(file, fresh)
	file.Seek(0, 0)

	if err == nil {
		log.Println("download succeeded.")
	}

	return file, err
}

func (t *RFC1123Time) UnmarshalJSON(b []byte) (err error) {
	var str string
	if err = json.Unmarshal(b, &str); err != nil {
		return err
	}
	t.Time, err = time.Parse(time.RFC1123, str)
	return err
}

func rmsListHandle(args []string) {
	set := pflag.NewFlagSet("rmslist", pflag.ExitOnError)
	mode := set.StringP("mode", "m", "", "")
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

	file, err := GetGatewayStatusCached(filePath, *forceDownload)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	var status GatewayStatus

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
			if mode != nil && !strings.Contains(strings.ToLower(channel.SupportedModes), *mode) {
				continue
			}
			printed = true

			freq := Frequency(channel.Frequency)
			dial := freq.Dial(channel.SupportedModes)

			url := channel.URL(gw.Callsign)

			fmt.Printf(fmtStr, gw.Callsign, channel.Gridsquare, channel.SupportedModes, dial, freq, url)
		}
		if printed {
			fmt.Println("")
		}
	}
}

func (gc GatewayChannel) URL(targetcall string) *url.URL {
	freq := Frequency(gc.Frequency).Dial(gc.SupportedModes)

	url, _ := url.Parse(fmt.Sprintf("%s:///%s?freq=%v", gc.Transport(), targetcall, freq.KHz()))
	return url
}

func (gc GatewayChannel) Transport() string {
	modes := strings.ToLower(gc.SupportedModes)
	switch {
	case strings.Contains(modes, "winmor"):
		return "winmor"
	case strings.Contains(modes, "packet"):
		return "ax25"
	case strings.Contains(modes, "pactor"):
		return "pactor"
	default:
		return ""
	}
}
