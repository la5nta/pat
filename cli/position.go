package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/la5nta/pat/app"
	"github.com/la5nta/pat/internal/gpsd"
	"github.com/la5nta/wl2k-go/catalog"
	"github.com/spf13/pflag"
)

var ExamplePosition = `
  position -c "QRV 145.500MHz"       Send position and comment with coordinates retrieved from GPSd.
  position --latlon 59.123,005.123   Send position 59.123N 005.123E.
  position --latlon 40.704,-73.945   Send position 40.704N 073.945W.
  position --latlon -10.123,-60.123  Send position 10.123S 060.123W.
`

func PositionHandle(ctx context.Context, app *app.App, args []string) {
	var latlon, comment string

	set := pflag.NewFlagSet("position", pflag.ExitOnError)
	set.StringVar(&latlon, "latlon", "", "")
	set.StringVarP(&comment, "comment", "c", "", "")
	set.Parse(args)

	report := catalog.PosReport{Comment: comment}

	if latlon != "" {
		parts := strings.Split(latlon, ",")
		if len(parts) != 2 {
			log.Fatal(`Invalid position format. Expected "latitude,longitude".`)
		}

		lat, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			log.Fatal(err)
		}
		report.Lat = &lat

		lon, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			log.Fatal(err)
		}
		report.Lon = &lon
	} else if app.Config().GPSd.Addr != "" {
		conn, err := gpsd.Dial(app.Config().GPSd.Addr)
		if err != nil {
			log.Fatalf("GPSd daemon: %s", err)
		}
		defer conn.Close()
		conn.Watch(true)

		posChan := make(chan gpsd.Position)
		go func() {
			defer close(posChan)
			pos, err := conn.NextPos()
			if err != nil {
				log.Printf("GPSd: %s", err)
				return
			}
			posChan <- pos
		}()

		log.Println("Waiting for position from GPSd...") // TODO: Spinning bar?
		var pos gpsd.Position
		select {
		case p := <-posChan:
			pos = p
		case <-ctx.Done():
			log.Println("Cancelled")
			return
		}
		report.Lat = &pos.Lat
		report.Lon = &pos.Lon
		if app.Config().GPSd.UseServerTime {
			report.Date = time.Now()
		} else {
			report.Date = pos.Time
		}

		// Course and speed is part of the spec, but does not seem to be
		// supported by winlink.org anymore. Ignore it for now.
		if false && pos.Track != 0 {
			course := CourseFromFloat64(pos.Track, false)
			report.Course = &course
		}
	} else {
		fmt.Println("No position available. See --help")
		os.Exit(1)
	}

	if report.Date.IsZero() {
		report.Date = time.Now()
	}

	postMessage(app, report.Message(app.Options().MyCall))
}

func CourseFromFloat64(f float64, magnetic bool) catalog.Course {
	c := catalog.Course{Magnetic: magnetic}

	str := fmt.Sprintf("%03.0f", f)
	for i := 0; i < 3; i++ {
		c.Digits[i] = str[i]
	}

	return c
}
