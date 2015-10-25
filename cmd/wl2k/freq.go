// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
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

type Frequency int

func (f Frequency) String() string {
	m := f / 1e6
	k := (float64(f) - float64(m)*1e6) / 1e3

	return fmt.Sprintf("%d.%06.2f MHz", m, k)
}

func (f Frequency) Dial(mode string) Frequency {
	mode = strings.ToLower(mode)

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

func freq(param string) {
	parts := strings.SplitN(param, ":", 2)

	var rig hamlib.Rig
	var ok bool
	switch parts[0] {
	case MethodWinmor:
		rig, ok = rigs[config.Winmor.Rig]
	case MethodArdop:
		rig, ok = rigs[config.Ardop.Rig]	
	case "":
		fmt.Println("Need freq method.")
		return
	default:
		fmt.Printf("'%s' not a supported freq method.\n", parts[0])
		return
	}

	if !ok {
		log.Printf("Hamlib rig not loaded.")
		return
	}

	if len(parts) < 2 {
		freq, err := rig.CurrentVFO().GetFreq()
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

func setFreq(rig hamlib.Rig, freq string) (newFreq, oldFreq int, err error) {
	oldFreq, err = rig.CurrentVFO().GetFreq()
	if err != nil {
		return 0, 0, fmt.Errorf("Unable to get rig frequency: %s", err)
	}

	f, err := strconv.ParseFloat(freq, 32)
	if err != nil {
		return 0, 0, err
	}

	newFreq = int(f * 1e3)
	err = rig.CurrentVFO().SetFreq(newFreq)
	return
}
