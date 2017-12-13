// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"log"
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

type Frequency int

func (f Frequency) String() string {
	m := f / 1e6
	k := (float64(f) - float64(m)*1e6) / 1e3

	return fmt.Sprintf("%d.%06.2f MHz", m, k)
}

func (f Frequency) KHz() float64 { return float64(f) / 1e3 }

func (f Frequency) Dial(mode string) Frequency {
	mode = strings.ToLower(mode)

	// Try to detect FM modes
	// (ARDOP on FM is reported as `ARDOP 2000 FM`)
	if strings.HasSuffix(mode, "fm") {
		return f
	}

	offsets := map[string]Frequency{
		"winmor": 1500,
		"pactor": 1500,
		"ardop":  1500,
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

func VFOForTransport(transport string) (vfo hamlib.VFO, ok bool) {
	switch transport {
	case MethodWinmor:
		vfo, ok = rigs[config.Winmor.Rig]
	case MethodArdop:
		vfo, ok = rigs[config.Ardop.Rig]
	case MethodAX25:
		vfo, ok = rigs[config.AX25.Rig]
	}
	return
}

func freq(param string) {
	parts := strings.SplitN(param, ":", 2)
	if parts[0] == "" {
		fmt.Println("Need freq method.")
	}

	rig, ok := VFOForTransport(parts[0])
	if !ok {
		log.Printf("Hamlib rig not loaded.")
		return
	}

	if len(parts) < 2 {
		freq, err := rig.GetFreq()
		if err != nil {
			log.Printf("Unable to get frequency: %s", err)
		}
		fmt.Printf("%.3f\n", float64(freq)/1e3)
		return
	}

	if _, _, err := setFreq(rig, parts[1]); err != nil {
		log.Printf("Unable to set frequency: %s", err)
	}
}

func setFreq(rig hamlib.VFO, freq string) (newFreq, oldFreq int, err error) {
	oldFreq, err = rig.GetFreq()
	if err != nil {
		return 0, 0, fmt.Errorf("Unable to get rig frequency: %s", err)
	}

	f, err := strconv.ParseFloat(freq, 32)
	if err != nil {
		return 0, 0, err
	}

	newFreq = int(f * 1e3)
	err = rig.SetFreq(newFreq)
	return
}
