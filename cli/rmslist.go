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
	if *byDistance {
		sort.Sort(app.ByDist(rList))
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
