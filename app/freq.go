// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package app

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/la5nta/wl2k-go/rigcontrol/hamlib"
)

var bands = map[string]Band{
	"160m":  {1.8e6, 2.0e6},
	"80m":   {3.5e6, 4.0e6},
	"60m":   {5.2e6, 5.5e6},
	"40m":   {7.0e6, 7.3e6},
	"30m":   {10.1e6, 10.2e6},
	"20m":   {14.0e6, 14.4e6},
	"17m":   {18.0e6, 18.2e6},
	"15m":   {21.0e6, 21.5e6},
	"12m":   {24.8e6, 25.0e6},
	"10m":   {28.0e6, 30.0e6},
	"6m":    {50.0e6, 54.0e6},
	"4m":    {70.0e6, 70.5e6},
	"2m":    {144.0e6, 148.0e6},
	"1.25m": {219.0e6, 225.0e6}, // 220, 222 (MHz)
	"70cm":  {420.0e6, 450.0e6},
}

type Band struct{ lower, upper Frequency }

func (b Band) Contains(f Frequency) bool {
	if b.lower == 0 && b.upper == 0 {
		return true
	}
	return f >= b.lower && f <= b.upper
}

type Frequency int // Hz

func (f Frequency) String() string {
	m := f / 1e6
	k := (float64(f) - float64(m)*1e6) / 1e3

	return fmt.Sprintf("%d.%06.2f MHz", m, k)
}

func (f Frequency) MarshalJSON() ([]byte, error) {
	type obj struct {
		Hz   json.Number `json:"hz"`
		KHz  json.Number `json:"khz"`
		Desc string      `json:"desc"`
	}
	return json.Marshal(obj{
		Hz:   json.Number(fmt.Sprint(int(f))),
		KHz:  json.Number(fmt.Sprint(f.KHz())),
		Desc: f.String(),
	})
}

func (f Frequency) KHz() float64 { return float64(f) / 1e3 }
func (f Frequency) MHz() float64 { return float64(f) / 1e6 }

func (f Frequency) Dial(mode string) Frequency {
	mode = strings.ToLower(mode)

	// Try to detect FM modes, e.g. `ARDOP 2000 FM` and `VARA FM WIDE`
	if strings.Contains(mode, "fm") {
		return f
	}

	offsets := map[string]Frequency{
		MethodPactor: 1500,
		MethodArdop:  1500,
		// varahf doesn't appear in RMS list from WDT
		"vara": 1500,
	}

	var shift Frequency
	for m, offset := range offsets {
		if strings.Contains(mode, m) {
			shift = -offset
			break
		}
	}

	return f + shift
}

func SetFreq(rig hamlib.VFO, freq string) (newFreq, oldFreq int, err error) {
	oldFreq, err = rig.GetFreq()
	if err != nil {
		return 0, 0, fmt.Errorf("unable to get rig frequency: %w", err)
	}

	f, err := strconv.ParseFloat(freq, 64)
	if err != nil {
		return 0, 0, err
	}

	newFreq = int(f * 1e3)
	err = rig.SetFreq(newFreq)
	return
}
