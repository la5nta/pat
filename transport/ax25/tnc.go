// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package ax25

import (
	"strings"
	"time"
)

const (
	B9600 Baudrate = 9600
	B1200          = 1200
)

const (
	_CONFIG_TXDELAY_UNIT       = time.Millisecond * 10
	_CONFIG_SLOT_TIME_UNIT     = time.Millisecond * 10
	_CONFIG_FRACK_UNIT         = time.Second
	_CONFIG_RESPONSE_TIME_UNIT = time.Millisecond * 100
)

type tncAddr struct {
	address Address
	digis   []Address
}

func (a tncAddr) Digis() []Address { return a.digis }
func (a tncAddr) Address() Address { return a.address }

type Baudrate int

type Config struct {
	HBaud        Baudrate      // Baudrate for packet channel [1200/9600].
	TXDelay      time.Duration // Time delay between PTT ON and start of transmission [(0 - 120) * 10ms].
	PacketLength uint8         // Maximum length of the data portion of a packet [0 - 255 bytes].
	Persist      uint8         // Parameter to calculate probability for the PERSIST/SLOTTIME method [0-255].
	SlotTime     time.Duration // Period of random number generation intervals for the PERSIST/SLOTTIME method [0-255 * 10ms].
	MaxFrame     uint8         // Maximum number of packets to be transmitted at one time.
	FRACK        time.Duration // Interval from one transmission until retry of transmission [0-250 * 1s].
	ResponseTime time.Duration // ACK-packet transmission delay [0-255 * 100ms].
}

func NewConfig(baud Baudrate) Config {
	switch baud {
	case B1200:
		return Config{
			HBaud:        B1200,
			TXDelay:      120 * time.Millisecond,
			PacketLength: 128,
			Persist:      128,
			SlotTime:     50 * time.Millisecond,
			MaxFrame:     6,
			FRACK:        5 * time.Second,
			ResponseTime: 100 * time.Millisecond,
		}
	case B9600:
		return Config{
			HBaud:        B9600,
			TXDelay:      100 * time.Millisecond,
			PacketLength: 255,
			Persist:      190,
			SlotTime:     50 * time.Millisecond,
			MaxFrame:     6, // Everything but 1 is outside range on TH-D72
			FRACK:        5 * time.Second,
			ResponseTime: 0 * time.Millisecond,
		}
	}
	return Config{}
}

//TODO:review and improve
func tncAddrFromString(str string) tncAddr {
	parts := strings.Split(str, " ")
	addr := tncAddr{
		address: AddressFromString(parts[0]),
		digis:   make([]Address, 0),
	}
	if len(parts) < 3 || !(parts[1] == "via" || parts[1] == "v") {
		return addr
	}
	parts = parts[2:]

	// Parse digis
	for _, dpart := range parts {
		addr.digis = append(addr.digis, AddressFromString(dpart))
	}
	return addr
}
