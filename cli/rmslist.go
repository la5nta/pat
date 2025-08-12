package cli

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/la5nta/pat/app"
	"github.com/spf13/pflag"
)

func RMSListHandle(ctx context.Context, a *app.App, args []string) {
	cancel := exitOnContextCancellation(ctx)
	defer cancel()

	set := pflag.NewFlagSet("rmslist", pflag.ExitOnError)
	mode := set.StringP("mode", "m", "", "")
	band := set.StringP("band", "b", "", "")
	forceDownload := set.BoolP("force-download", "d", false, "")
	byDistance := set.BoolP("sort-distance", "s", false, "")
	byLinkQuality := set.BoolP("sort-link-quality", "q", false, "Sort by predicted link quality")
	set.Parse(args)

	var query string
	if len(set.Args()) > 0 {
		query = strings.ToUpper(set.Args()[0])
	}

	*mode = strings.ToLower(*mode)
	rList, err := a.ReadRMSList(ctx, *forceDownload, func(rms app.RMS) bool {
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
	switch {
	case *byDistance:
		sort.Sort(app.ByDist(rList))
	case *byLinkQuality:
		sort.Sort(sort.Reverse(app.ByLinkQuality(rList)))
	}

	fmtStr := "%-9.9s [%-6.6s] %-6.6s %3.3s %-15.15s %14.14s %14.14s %5.5s %s\n"

	// Print header
	fmt.Printf(fmtStr, "callsign", "gridsq", "dist", "Az", "mode(s)", "dial freq", "center freq", "qual", "url")

	// Print gateways (separated by blank line)
	for i, r := range rList {
		qual := "N/A"
		if r.Prediction != nil {
			qual = fmt.Sprintf("%d%%", r.Prediction.LinkQuality)
		}
		printRMS(r, qual)
		if i+1 < len(rList) && rList[i].Callsign != rList[i+1].Callsign {
			fmt.Println("")
		}
	}
}

func printRMS(r app.RMS, qual string) {
	fmtStr := "%-9.9s [%-6.6s] %-6.6s %3.3s %-15.15s %14.14s %14.14s %5.5s %s\n"
	distance := strconv.FormatFloat(float64(r.Distance), 'f', 0, 64)
	azimuth := strconv.FormatFloat(float64(r.Azimuth), 'f', 0, 64)
	url := ""
	if r.URL != nil {
		url = r.URL.String()
	}
	fmt.Printf(fmtStr, r.Callsign, r.Gridsquare, distance, azimuth, r.Modes, r.Dial, r.Freq, qual, url)
}
