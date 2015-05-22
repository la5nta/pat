// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// Package catalog provides helpers for using the Winlink 2000 catalog services.
package catalog

import (
	"bytes"
	"fmt"
	"math"
	"time"

	"github.com/la5nta/wl2k-go"
)

type PosReport struct {
	Date     time.Time
	Lat, Lon *float64 // In decimal degrees
	Speed    *float64 // Unit not specified in winlink docs
	Course   *Course
	Comment  string // Up to 80 characters
}

type Course struct {
	Digits   [3]byte
	Magnetic bool
}

func (c Course) String() string {
	if c.Magnetic {
		return fmt.Sprintf("%sM", string(c.Digits[:3]))
	} else {
		return fmt.Sprintf("%sT", string(c.Digits[:3]))
	}
}

func (p PosReport) Message(from string) *wl2k.Message {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "DATE: %s\r\n", p.Date.UTC().Format(wl2k.DateLayout))

	if p.Lat != nil && p.Lon != nil {
		fmt.Fprintf(&buf, "LATITUDE: %s\r\n", decToMinDec(*p.Lat, true))
		fmt.Fprintf(&buf, "LONGITUDE: %s\r\n", decToMinDec(*p.Lon, false))
	}
	if p.Speed != nil {
		fmt.Fprintf(&buf, "SPEED: %f\r\n", *p.Speed)
	}
	if p.Course != nil {
		fmt.Fprintf(&buf, "COURSE: %s\r\n", *p.Course)
	}
	if len(p.Comment) > 0 {
		fmt.Fprintf(&buf, "COMMENT: %s\r\n", p.Comment)
	}

	return &wl2k.Message{
		MID:     wl2k.GenerateMid(from),
		To:      []wl2k.Address{wl2k.Address{Addr: "QTH"}},
		From:    wl2k.AddressFromString(from),
		Mbo:     from,
		Date:    time.Now(),
		Type:    "Position Report",
		Subject: "POSITION REPORT",
		Body:    wl2k.Body(buf.Bytes()),
	}
}

// Format: 23-42.3N
func decToMinDec(dec float64, latitude bool) string {
	deg := int(dec)
	min := (dec - float64(deg)) * 60.0

	var sign byte
	if latitude && deg > 0 {
		sign = 'N'
	} else if latitude && deg < 0 {
		sign = 'S'
	} else if !latitude && deg > 0 {
		sign = 'E'
	} else if !latitude && deg < 0 {
		sign = 'W'
	} else {
		sign = ' '
	}

	return fmt.Sprintf("%02.0f-%.4f%c", math.Abs(float64(deg)), math.Abs(min), sign)
}
