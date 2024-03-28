package forms

import (
	"fmt"
	"math"

	"github.com/la5nta/pat/internal/gpsd"
	"github.com/pd0mz/go-maidenhead"
)

type gpsStyle int

const (
	// documentation: https://www.winlink.org/sites/default/files/RMSE_FORMS/insertion_tags.zip
	signedDecimal gpsStyle = iota // 41.1234 -73.4567
	decimal                       // 46.3795N 121.5835W
	degreeMinute                  // 46-22.77N 121-35.01W
	gridSquare                    // JO29PJ
)

func positionFmt(style gpsStyle, pos gpsd.Position) string {
	const notAvailable = "(Not available)"

	var (
		northing   string
		easting    string
		latDegrees int
		latMinutes float64
		lonDegrees int
		lonMinutes float64
	)

	noPos := gpsd.Position{}
	if pos == noPos {
		return notAvailable
	}
	switch style {
	case gridSquare:
		str, err := maidenhead.NewPoint(pos.Lat, pos.Lon).GridSquare()
		if err != nil {
			return notAvailable
		}
		return str
	case degreeMinute:
		{
			latDegrees = int(math.Trunc(math.Abs(pos.Lat)))
			latMinutes = (math.Abs(pos.Lat) - float64(latDegrees)) * 60
			lonDegrees = int(math.Trunc(math.Abs(pos.Lon)))
			lonMinutes = (math.Abs(pos.Lon) - float64(lonDegrees)) * 60
		}
		fallthrough
	case decimal:
		{
			if pos.Lat >= 0 {
				northing = "N"
			} else {
				northing = "S"
			}
			if pos.Lon >= 0 {
				easting = "E"
			} else {
				easting = "W"
			}
		}
	}

	switch style {
	case signedDecimal:
		return fmt.Sprintf("%.4f %.4f", pos.Lat, pos.Lon)
	case decimal:
		return fmt.Sprintf("%.4f%s %.4f%s", math.Abs(pos.Lat), northing, math.Abs(pos.Lon), easting)
	case degreeMinute:
		return fmt.Sprintf("%02d-%05.2f%s %03d-%05.2f%s", latDegrees, latMinutes, northing, lonDegrees, lonMinutes, easting)
	default:
		panic("invalid style")
	}
}
