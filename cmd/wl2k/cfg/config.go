// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package cfg

type Config struct {
	// This station's callsign.
	MyCall string `json:"mycall"`

	// Secure login password used when a secure login challenge is received.
	//
	// The user is prompted if this is undefined.
	SecureLoginPassword string `json:"secure_login_password"`

	// Auxiliary callsigns to fetch email on behalf of.
	AuxAddrs []string `json:"auxiliary_addresses"`

	// Maidenhead grid square (e.g. JP20qe).
	Locator string `json:"locator"`

	// Handshake comment lines sent to remote node on incoming connections.
	//
	// Example: ["QTH: Hagavik, Norway. Operator: Martin", "Rig: FT-897 with Signalink USB"]
	MOTD []string `json:"motd"`

	// Connect aliases,
	//
	// Alias a list of connect commands, which will be attempted in order until a successfull message exchange has been made.
	// Recursion is supported.
	//
	// Example: {"offgrid": ["winmor:LA3F@5350", "ax25:LA1J-10"], "NorwayHF": ["winmor:LA1B@5320", "winmor":"LA3F@3602", "winmor":"LA1J@5352"]}
	ConnectAliases map[string][]string `json:"connect_aliases"`

	// Methods to listen for incoming P2P connections by default.
	//
	// Example: ["ax25", "winmor", "telnet", "ardop"]
	Listen []string `json:"listen"`

	// Hamlib rigs available (with reference name) for ptt and frequency control.
	HamlibRigs map[string]HamlibConfig `json:"hamlib_rigs"`

	AX25      AX25Config      `json:"ax25"`       // See AX25Config.
	SerialTNC SerialTNCConfig `json:"serial-tnc"` // See SerialTNCConfig.
	Winmor    WinmorConfig    `json:"winmor"`     // See WinmorConfig.
	Ardop    ArdopConfig    `json:"ardop"`     // See ArdopConfig.
	Telnet    TelnetConfig    `json:"telnet"`     // See TelnetConfig.

	// Command schedule (cron-like syntax).
	//
	// Examples:
	//   # Connect to telnet once every hour
	//   "@hourly": "connect telnet"
	//
	//   # Change winmor listen frequency based on hour of day
	//   "00 10 * * *": "freq winmor:7350.000", # 40m from 10:00
	//   "00 18 * * *": "freq winmor:5347.000", # 60m from 18:00
	//   "00 22 * * *": "freq winmor:3602.000"  # 80m from 22:00
	Schedule map[string]string `json:"schedule"`
}

type HamlibConfig struct {
	// The network type ("serial" or "tcp"). Use 'tcp' for rigctld.
	//
	// (For serial support: build with "-tags libhamlib".)
	Network string `json:"network,omitempty"`

	// The rig address.
	//
	// For tcp (rigctld): "address:port" (e.g. localhost:4532).
	// For serial: "/path/to/tty?model=&baudrate=" (e.g. /dev/ttyS0?model=123&baudrate=4800).
	Address string `json:"address,omitempty"`
}

type WinmorConfig struct {
	// Network address of the Winmor TNC (e.g. localhost:8500).
	Addr string `json:"addr"`

	// Bandwidth to use when getting an inbound connection (500/1600).
	InboundBandwidth int `json:"inbound_bandwidth"`

	// (optional) Reference name to the Hamlib rig to control frequency and ptt.
	Rig string `json:"rig"` //TODO: Maybe custom unmarshal to ensure the rig is registered?

	// Set to true if hamlib should control PTT (SignaLink=false, most rigexpert=true).
	PTTControl bool `json:"ptt_ctrl"`
}

type ArdopConfig struct {
	// Network address of the Ardop TNC (e.g. localhost:8515).
	Addr string `json:"addr"`

	// Bandwidth to use when getting an inbound connection (500/1600).
	InboundBandwidth int `json:"inbound_bandwidth"`

	// (optional) Reference name to the Hamlib rig to control frequency and ptt.
	Rig string `json:"rig"` //TODO: Maybe custom unmarshal to ensure the rig is registered?

	// Set to true if hamlib should control PTT (SignaLink=false, most rigexpert=true).
	PTTControl bool `json:"ptt_ctrl"`
}

type TelnetConfig struct {
	// Network address (and port) to listen for telnet-p2p connections (e.g. :8774).
	ListenAddr string `json:"listen_addr"`

	// Telnet-p2p password.
	Password string `json:"password"`
}

type SerialTNCConfig struct {
	// Serial port (e.g. /dev/ttyUSB0 or COM1).
	Path string `json:"path"`

	// Baudrate for the serial port (e.g. 57600).
	Baudrate int `json:"baudrate"`

	// Type of TNC (currently only 'kenwood').
	Type string `json:"type"`
}

type AX25Config struct {
	// axport to use (as defined in /etc/ax25/axports).
	Port string `json:"port"`

	// Optional beacon when listening for incoming packet-p2p connections.
	Beacon BeaconConfig `json:"beacon"`
}

type BeaconConfig struct {
	// Beacon interval in seconds (e.g. 3600 for once every 1 hour)
	Every int `json:"every"` // (seconds)

	// Beacon data/message
	Message string `json:"message"`

	// Beacon destination (e.g. IDENT)
	Destination string `json:"destination"`
}

var DefaultConfig Config = Config{
	MyCall:         "",
	MOTD:           []string{"Open source WL2K client - github.com/LA5NTA/wl2k-go"},
	AuxAddrs:       []string{},
	ConnectAliases: map[string][]string{},
	AX25: AX25Config{
		Beacon: BeaconConfig{
			Every:       3600,
			Message:     "Winlink P2P",
			Destination: "IDENT",
		},
	},
	SerialTNC: SerialTNCConfig{
		Path:     "/dev/ttyUSB0",
		Baudrate: 9600,
		Type:     "Kenwood",
	},
	Winmor: WinmorConfig{
		Addr:             "localhost:8500",
		InboundBandwidth: 1600,
	},
	Ardop: ArdopConfig{
		Addr:             "localhost:8515",
		InboundBandwidth: 1600,
	},	
	Telnet: TelnetConfig{
		ListenAddr: ":8774",
		Password:   "",
	},
}
