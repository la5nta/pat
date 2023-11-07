// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package cfg

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/la5nta/wl2k-go/transport/ardop"
)

const (
	PlaceholderMycall = "{mycall}"
)

type AuxAddr struct {
	Address  string
	Password *string
}

func (a AuxAddr) MarshalJSON() ([]byte, error) {
	if a.Password == nil {
		return json.Marshal(a.Address)
	}
	return json.Marshal(a.Address + ":" + *a.Password)
}

func (a *AuxAddr) UnmarshalJSON(p []byte) error {
	var str string
	if err := json.Unmarshal(p, &str); err != nil {
		return err
	}
	parts := strings.SplitN(str, ":", 2)
	a.Address = parts[0]
	if len(parts) > 1 {
		a.Password = &parts[1]
	}
	return nil
}

type Config struct {
	// This station's callsign.
	MyCall string `json:"mycall"`

	// Secure login password used when a secure login challenge is received.
	//
	// The user is prompted if this is undefined.
	SecureLoginPassword string `json:"secure_login_password"`

	// Auxiliary callsigns to fetch email on behalf of.
	//
	// Passwords can optionally be specified by appending :MYPASS (e.g. EMCOMM-1:MyPassw0rd).
	// If no password is specified, the SecureLoginPassword is used.
	AuxAddrs []AuxAddr `json:"auxiliary_addresses"`

	// Maidenhead grid square (e.g. JP20qe).
	Locator string `json:"locator"`

	// List of service codes for rmslist (defaults to PUBLIC)
	ServiceCodes []string `json:"service_codes"`

	// Default HTTP listen address (for web UI).
	//
	// Use ":8080" to listen on any device, port 8080.
	HTTPAddr string `json:"http_addr"`

	// Handshake comment lines sent to remote node on incoming connections.
	//
	// Example: ["QTH: Hagavik, Norway. Operator: Martin", "Rig: FT-897 with Signalink USB"]
	MOTD []string `json:"motd"`

	// Connect aliases
	//
	// Example: {"LA1B-10": "ax25:///LD5GU/LA1B-10", "LA1B": "ardop://LA3F?freq=5350"}
	// Any occurrence of the substring "{mycall}" will be replaced with user's callsign.
	ConnectAliases map[string]string `json:"connect_aliases"`

	// Methods to listen for incoming P2P connections by default.
	//
	// Example: ["ax25", "telnet", "ardop"]
	Listen []string `json:"listen"`

	// Hamlib rigs available (with reference name) for ptt and frequency control.
	HamlibRigs map[string]HamlibConfig `json:"hamlib_rigs"`

	AX25      AX25Config      `json:"ax25"`       // See AX25Config.
	AX25Linux AX25LinuxConfig `json:"ax25_linux"` // See AX25LinuxConfig.
	AGWPE     AGWPEConfig     `json:"agwpe"`      // See AGWPEConfig.
	SerialTNC SerialTNCConfig `json:"serial-tnc"` // See SerialTNCConfig.
	Ardop     ArdopConfig     `json:"ardop"`      // See ArdopConfig.
	Pactor    PactorConfig    `json:"pactor"`     // See PactorConfig.
	Telnet    TelnetConfig    `json:"telnet"`     // See TelnetConfig.
	VaraHF    VaraConfig      `json:"varahf"`     // See VaraConfig.
	VaraFM    VaraConfig      `json:"varafm"`     // See VaraConfig.

	// See GPSdConfig.
	GPSd GPSdConfig `json:"gpsd"`

	// Legacy support for old config files only. This field is deprecated!
	// Please use "Addr" field in GPSd config struct (GPSd.Addr)
	GPSdAddrLegacy string `json:"gpsd_addr,omitempty"`

	// Command schedule (cron-like syntax).
	//
	// Examples:
	//   # Connect to telnet once every hour
	//   "@hourly": "connect telnet"
	//
	//   # Change ardop listen frequency based on hour of day
	//   "00 10 * * *": "freq ardop:7350.000", # 40m from 10:00
	//   "00 18 * * *": "freq ardop:5347.000", # 60m from 18:00
	//   "00 22 * * *": "freq ardop:3602.000"  # 80m from 22:00
	Schedule map[string]string `json:"schedule"`

	// By default, Pat posts your callsign and running version to the Winlink CMS Web Services
	//
	// Set to true if you don't want your information sent.
	VersionReportingDisabled bool `json:"version_reporting_disabled"`
}

type HamlibConfig struct {
	// The network type ("serial" or "tcp"). Use 'tcp' for rigctld (default).
	//
	// (For serial support: build with "-tags libhamlib".)
	Network string `json:"network,omitempty"`

	// The rig address.
	//
	// For tcp (rigctld): "address:port" (e.g. localhost:4532).
	// For serial: "/path/to/tty?model=&baudrate=" (e.g. /dev/ttyS0?model=123&baudrate=4800).
	Address string `json:"address,omitempty"`

	// The rig's VFO to control ("A" or "B"). If empty, the current active VFO is used.
	VFO string `json:"VFO"`
}

type ArdopConfig struct {
	// Network address of the Ardop TNC (e.g. localhost:8515).
	Addr string `json:"addr"`

	// Default/listen ARQ bandwidth (200/500/1000/2000 MAX/FORCED).
	ARQBandwidth ardop.Bandwidth `json:"arq_bandwidth"`

	// (optional) Reference name to the Hamlib rig to control frequency and ptt.
	Rig string `json:"rig"`

	// Set to true if hamlib should control PTT (SignaLink=false, most rigexpert=true).
	PTTControl bool `json:"ptt_ctrl"`

	// (optional) Send ID frame at a regular interval when the listener is active (unit is seconds)
	BeaconInterval int `json:"beacon_interval"`

	// Send FSK CW ID after an ID frame.
	CWID bool `json:"cwid_enabled"`
}

type VaraConfig struct {
	// Network host of the VARA modem (defaults to localhost:8300).
	Addr string `json:"addr"`

	// Default/listen bandwidth (HF: 500/2300/2750 Hz).
	Bandwidth int `json:"bandwidth"`

	// (optional) Reference name to the Hamlib rig to control frequency and ptt.
	Rig string `json:"rig"`

	// Set to true if hamlib should control PTT (SignaLink=false, most rigexpert=true).
	PTTControl bool `json:"ptt_ctrl"`
}

// UnmarshalJSON implements VaraConfig JSON unmarshalling with support for legacy format.
func (v *VaraConfig) UnmarshalJSON(b []byte) error {
	type newFormat VaraConfig
	legacy := struct {
		newFormat
		Host     string `json:"host"`
		CmdPort  int    `json:"cmdPort"`
		DataPort int    `json:"dataPort"`
	}{}
	if err := json.Unmarshal(b, &legacy); err != nil {
		return err
	}
	if legacy.newFormat.Addr == "" && legacy.Host != "" {
		legacy.newFormat.Addr = fmt.Sprintf("%s:%d", legacy.Host, legacy.CmdPort)
	}
	*v = VaraConfig(legacy.newFormat)
	if !v.IsZero() && v.CmdPort() <= 0 {
		return fmt.Errorf("invalid addr format")
	}
	return nil
}

func (v VaraConfig) IsZero() bool { return v == (VaraConfig{}) }

func (v VaraConfig) Host() string {
	host, _, _ := net.SplitHostPort(v.Addr)
	return host
}

func (v VaraConfig) CmdPort() int {
	_, portStr, _ := net.SplitHostPort(v.Addr)
	port, _ := strconv.Atoi(portStr)
	return port
}
func (v VaraConfig) DataPort() int { return v.CmdPort() + 1 }

type PactorConfig struct {
	// Path/port to TNC device (e.g. /dev/ttyUSB0 or COM1).
	Path string `json:"path"`

	// Baudrate for the serial port (e.g. 57600).
	Baudrate int `json:"baudrate"`

	// (optional) Reference name to the Hamlib rig for frequency control.
	Rig string `json:"rig"`

	// (optional) Path to custom TNC initialization script.
	InitScript string `json:"custom_init_script"`
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

	// SerialBaud is the serial port's baudrate (e.g. 57600).
	SerialBaud int `json:"serial_baud"`

	// HBaud is the the packet connection's baudrate (1200 or 9600).
	HBaud int `json:"hbaud"`

	// Baudrate of the packet connection.
	// Deprecated: Use HBaud instead.
	BaudrateLegacy int `json:"baudrate,omitempty"`

	// Type of TNC (currently only 'kenwood').
	Type string `json:"type"`

	// (optional) Reference name to the Hamlib rig for frequency control.
	Rig string `json:"rig"`
}

type AGWPEConfig struct {
	// The TCP address of the TNC.
	Addr string `json:"addr"`

	// The AGWPE "radio port" (0-3).
	RadioPort int `json:"radio_port"`
}

type AX25Config struct {
	// The AX.25 engine to be used.
	//
	// Valid options are:
	//   - linux
	//   - agwpe
	//   - serial-tnc
	Engine AX25Engine `json:"engine"`

	// (optional) Reference name to the Hamlib rig for frequency control.
	Rig string `json:"rig"`

	// DEPRECATED: See AX25Linux.Port.
	AXPort string `json:"port,omitempty"`

	// Optional beacon when listening for incoming packet-p2p connections.
	Beacon BeaconConfig `json:"beacon"`
}

type AX25LinuxConfig struct {
	// axport to use (as defined in /etc/ax25/axports). Only applicable to ax25 engine 'linux'.
	Port string `json:"port"`
}

type BeaconConfig struct {
	// Beacon interval in seconds (e.g. 3600 for once every 1 hour)
	Every int `json:"every"` // (seconds)

	// Beacon data/message
	Message string `json:"message"`

	// Beacon destination (e.g. IDENT)
	Destination string `json:"destination"`
}

type GPSdConfig struct {
	// Enable GPSd proxy for HTTP (web GUI)
	//
	// Caution: Your GPS position will be accessible to any network device able to access Pat's HTTP interface.
	EnableHTTP bool `json:"enable_http"`

	// Allow Winlink forms to use GPSd for aquiring your position.
	//
	// Caution: Your current GPS position will be automatically injected, without your explicit consent, into forms requesting such information.
	AllowForms bool `json:"allow_forms"`

	// Use server time instead of timestamp provided by GPSd (e.g for older GPS device with week roll-over issue).
	UseServerTime bool `json:"use_server_time"`

	// Address and port of GPSd server (e.g. localhost:2947).
	Addr string `json:"addr"`
}

var DefaultConfig = Config{
	MOTD:         []string{"Open source Winlink client - getpat.io"},
	AuxAddrs:     []AuxAddr{},
	ServiceCodes: []string{"PUBLIC"},
	ConnectAliases: map[string]string{
		"telnet": "telnet://{mycall}:CMSTelnet@cms.winlink.org:8772/wl2k",
	},
	Listen:   []string{},
	HTTPAddr: "localhost:8080",
	AX25: AX25Config{
		Engine: DefaultAX25Engine(),
		Beacon: BeaconConfig{
			Every:       3600,
			Message:     "Winlink P2P",
			Destination: "IDENT",
		},
	},
	AX25Linux: AX25LinuxConfig{
		Port: "wl2k",
	},
	SerialTNC: SerialTNCConfig{
		Path:       "/dev/ttyUSB0",
		SerialBaud: 9600,
		HBaud:      1200,
		Type:       "Kenwood",
	},
	AGWPE: AGWPEConfig{
		Addr:      "localhost:8000",
		RadioPort: 0,
	},
	Ardop: ArdopConfig{
		Addr:         "localhost:8515",
		ARQBandwidth: ardop.Bandwidth500Max,
		CWID:         true,
	},
	Pactor: PactorConfig{
		Path:     "/dev/ttyUSB0",
		Baudrate: 57600,
	},
	Telnet: TelnetConfig{
		ListenAddr: ":8774",
		Password:   "",
	},
	VaraHF: VaraConfig{
		Addr:      "localhost:8300",
		Bandwidth: 2300,
	},
	VaraFM: VaraConfig{
		Addr: "localhost:8300",
	},
	GPSd: GPSdConfig{
		EnableHTTP:    false, // Default to false to help protect privacy of unknowing users (see github.com//issues/146)
		AllowForms:    false, // Default to false to help protect location privacy of unknowing users
		UseServerTime: false,
		Addr:          "localhost:2947", // Default listen address for GPSd
	},
	GPSdAddrLegacy: "",
	Schedule:       map[string]string{},
	HamlibRigs:     map[string]HamlibConfig{},
}
