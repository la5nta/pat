// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// Package app implements the core functionality shared by cli and api.
package app

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/harenber/ptc-go/v2/pactor"
	"github.com/la5nta/pat/api/types"
	"github.com/la5nta/pat/cfg"
	"github.com/la5nta/pat/internal/buildinfo"
	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/pat/internal/directories"
	"github.com/la5nta/pat/internal/forms"
	"github.com/la5nta/wl2k-go/mailbox"
	"github.com/la5nta/wl2k-go/rigcontrol/hamlib"
	"github.com/la5nta/wl2k-go/transport"
	"github.com/la5nta/wl2k-go/transport/ardop"
	"github.com/la5nta/wl2k-go/transport/ax25"
	"github.com/la5nta/wl2k-go/transport/ax25/agwpe"
	"github.com/n8jja/Pat-Vara/vara"
)

const (
	MethodArdop  = "ardop"
	MethodTelnet = "telnet"
	MethodPactor = "pactor"
	MethodVaraHF = "varahf"
	MethodVaraFM = "varafm"

	MethodAX25          = "ax25"
	MethodAX25AGWPE     = MethodAX25 + "+agwpe"
	MethodAX25Linux     = MethodAX25 + "+linux"
	MethodAX25SerialTNC = MethodAX25 + "+serial-tnc"

	// TODO: Remove after some release cycles (2023-05-21)
	MethodSerialTNCDeprecated = "serial-tnc"
)

type Options struct {
	IgnoreBusy bool // Move to connect?
	SendOnly   bool // Move to connect?
	RadioOnly  bool

	Robust       bool
	MyCall       string
	Listen       string
	MailboxPath  string
	ConfigPath   string
	PrehooksPath string
	LogPath      string
	EventLogPath string
	FormsPath    string
}

type App struct {
	options  Options
	config   cfg.Config
	OnReload func()

	mbox     *mailbox.DirHandler
	formsMgr *forms.Manager

	exchangeChan   chan ex        // The channel that the exchange loop is listening on
	exchangeConn   net.Conn       // Pointer to the active session connection (exchange)
	dialing        *transport.URL // The connect URL currently being dialed (if any)
	dialCancelFunc func()         // Context cancellation function for aborting while dialing.
	listenHub      *ListenerHub

	promptHub    *PromptHub
	websocketHub WSHub

	// Persistent modem connections
	ardop  *ardop.TNC
	agwpe  *agwpe.TNCPort
	pactor *pactor.Modem
	varaHF *vara.Modem
	varaFM *vara.Modem

	rigs map[string]rig

	eventLog  *EventLogger
	logWriter io.WriteCloser
}

// A rig holds a VFO and a closer for the underlying rig connection.
type rig struct {
	hamlib.VFO
	io.Closer
}

func New(opts Options) *App {
	opts.MailboxPath = filepath.Clean(opts.MailboxPath)
	opts.FormsPath = filepath.Clean(opts.FormsPath)
	opts.ConfigPath = filepath.Clean(opts.ConfigPath)
	opts.LogPath = filepath.Clean(opts.LogPath)
	opts.EventLogPath = filepath.Clean(opts.EventLogPath)

	return &App{options: opts, websocketHub: noopWSSocket{}}
}

func (a *App) Mailbox() *mailbox.DirHandler { return a.mbox }

func (a *App) FormsManager() *forms.Manager { return a.formsMgr }

func (a *App) Config() cfg.Config { return a.config }

func (a *App) Options() Options { return a.options }

func (a *App) PromptHub() *PromptHub { return a.promptHub }

func (a *App) Reload() error {
	if a.OnReload == nil {
		return fmt.Errorf("reload not supported")
	}
	a.OnReload()
	return nil
}

func (a *App) VFOForRig(rig string) (hamlib.VFO, bool) { r, ok := a.rigs[rig]; return r, ok }

func (a *App) VFOForTransport(transport string) (vfo hamlib.VFO, rigName string, ok bool, err error) {
	var rig string
	switch {
	case transport == MethodArdop:
		rig = a.config.Ardop.Rig
	case transport == MethodAX25, strings.HasPrefix(transport, MethodAX25+"+"):
		rig = a.config.AX25.Rig
	case transport == MethodPactor:
		rig = a.config.Pactor.Rig
	case transport == MethodVaraHF:
		rig = a.config.VaraHF.Rig
	case transport == MethodVaraFM:
		rig = a.config.VaraFM.Rig
	default:
		return vfo, "", false, fmt.Errorf("not supported with transport '%s'", transport)
	}
	if rig == "" {
		return vfo, "", false, fmt.Errorf("missing rig reference in config section for %s", transport)
	}
	vfo, ok = a.VFOForRig(rig)
	return vfo, rig, ok, nil
}

func (a *App) EnableWebSocket(ctx context.Context, wsHub WSHub) error {
	a.websocketHub = wsHub
	a.promptHub.AddPrompter(wsHub)
	return nil
}

func (a *App) Run(ctx context.Context, cmd Command, args []string) {
	debug.Printf("Version: %s", buildinfo.VersionString())
	debug.Printf("Command: %s %v", cmd.Str, args)
	debug.Printf("Mailbox dir is\t'%s'", a.options.MailboxPath)
	debug.Printf("Forms dir is\t'%s'", a.options.FormsPath)
	debug.Printf("Config file is\t'%s'", a.options.ConfigPath)
	debug.Printf("Log file is \t'%s'", a.options.LogPath)
	debug.Printf("Event log file is\t'%s'", a.options.EventLogPath)
	directories.MigrateLegacyDataDir()

	a.listenHub = NewListenerHub(a)
	a.listenHub.websocketHub = a.websocketHub
	a.promptHub = NewPromptHub()

	// Skip initialization for some commands
	switch cmd.Str {
	case "configure", "version":
		cmd.HandleFunc(ctx, a, args)
		return
	}

	// Enable the GZIP extension experiment by default
	if _, ok := os.LookupEnv("GZIP_EXPERIMENT"); !ok {
		os.Setenv("GZIP_EXPERIMENT", "1")
	}

	os.Setenv("PATH", fmt.Sprintf(`%s%c%s`, a.options.PrehooksPath, os.PathListSeparator, os.Getenv("PATH")))

	// Parse configuration file
	var err error
	a.config, err = LoadConfig(a.options.ConfigPath, cfg.DefaultConfig)
	if err != nil {
		log.Fatalf("Unable to load/write config: %s", err)
	}

	// Initialize logger
	f, err := os.Create(a.options.LogPath)
	if err != nil {
		log.Fatalf("Unable to create log file at %s: %v", a.options.LogPath, err)
	}
	a.logWriter = struct {
		io.Writer
		io.Closer
	}{io.MultiWriter(f, os.Stdout), f}
	log.SetOutput(a.logWriter)

	a.eventLog, err = NewEventLogger(a.options.EventLogPath)
	if err != nil {
		log.Fatal("Unable to open event log file:", err)
	}

	// Read command line options from config if unset
	if a.options.MyCall == "" && a.config.MyCall == "" {
		fmt.Fprint(os.Stderr, "Missing mycall\n")
		if cmd.Str != "http" {
			os.Exit(1)
		}
	} else if a.options.MyCall == "" {
		a.options.MyCall = a.config.MyCall
	}

	// Ensure mycall is all upper case.
	a.options.MyCall = strings.ToUpper(a.options.MyCall)

	// Don't use config password if we don't use config mycall
	if !strings.EqualFold(a.options.MyCall, a.config.MyCall) {
		a.config.SecureLoginPassword = ""
	}

	// Replace placeholders in connect aliases
	for k, v := range a.config.ConnectAliases {
		a.config.ConnectAliases[k] = strings.ReplaceAll(v, cfg.PlaceholderMycall, a.options.MyCall)
	}

	if a.options.Listen == "" && len(a.config.Listen) > 0 {
		a.options.Listen = strings.Join(a.config.Listen, ",")
	}

	// init forms subsystem
	a.formsMgr = forms.NewManager(forms.Config{
		FormsPath:      a.options.FormsPath,
		SequencePath:   filepath.Join(directories.StateDir(), "template-sequence-number.json"),
		SequenceFormat: "%03d",
		MyCall:         a.options.MyCall,
		Locator:        a.config.Locator,
		AppVersion:     buildinfo.AppName + " " + buildinfo.VersionStringShort(),
		UserAgent:      buildinfo.UserAgent(),
		GPSd:           a.config.GPSd,
	})

	// Load the mailbox handler
	a.mbox = mailbox.NewDirHandler(
		filepath.Join(a.options.MailboxPath, a.options.MyCall),
		a.options.SendOnly,
	)
	// Ensure the mailbox handler is ready
	if err := a.mbox.Prepare(); err != nil {
		log.Fatal(err)
	}

	if cmd.MayConnect {
		a.loadHamlibRigs(a.config.HamlibRigs)
		a.exchangeChan = a.exchangeLoop(ctx)

		go func() {
			if a.config.VersionReportingDisabled {
				return
			}
			for {
				a.postVersionUpdate()                  // 24 hour hold on success
				a.checkPasswordRecoveryEmailIsSet(ctx) // 14 day hold on success
				select {
				case <-time.After(6 * time.Hour): // Retry every 6 hours
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	if cmd.LongLived {
		if a.options.Listen != "" {
			a.Listen(a.options.Listen)
		}
	}

	// Start command execution
	cmd.HandleFunc(ctx, a, args)
}

type Heard struct {
	Callsign string    `json:"callsign"`
	Time     time.Time `json:"time"`
}

func (a *App) ActiveListeners() []string {
	slice := []string{}
	for _, tl := range a.listenHub.Active() {
		slice = append(slice, tl.Name())
	}
	sort.Strings(slice)
	return slice
}

func (a *App) Heard() map[string][]Heard {
	heard := make(map[string][]Heard)
	if a.ardop != nil {
		for callsign, time := range a.ardop.Heard() {
			heard[MethodArdop] = append(heard[MethodArdop], Heard{
				Callsign: callsign,
				Time:     time,
			})
		}
	}

	if ax25, err := ax25.Heard(a.Config().AX25Linux.Port); err != nil {
		for callsign, time := range ax25 {
			heard[MethodAX25Linux] = append(heard[MethodAX25Linux], Heard{
				Callsign: callsign,
				Time:     time,
			})
		}
	}

	return heard
}

func (a *App) GetStatus() types.Status {
	configHash := func(c cfg.Config) string {
		h := sha1.New()
		if err := json.NewEncoder(h).Encode(c); err != nil {
			panic(err)
		}
		return fmt.Sprintf("%x", h.Sum(nil))
	}

	status := types.Status{
		ActiveListeners: a.ActiveListeners(),
		Dialing:         a.dialing != nil,
		Connected:       a.exchangeConn != nil,
		HTTPClients:     a.websocketHub.ClientAddrs(),
		ConfigHash:      configHash(a.config),
	}

	if a.exchangeConn != nil {
		addr := a.exchangeConn.RemoteAddr()
		status.RemoteAddr = fmt.Sprintf("%s:%s", addr.Network(), addr)
	}

	return status
}

func (a *App) Close() {
	debug.Printf("Starting cleanup")
	defer func() {
		debug.Printf("Cleanup done")
		if a.logWriter != nil {
			a.logWriter.Close()
		}
	}()

	debug.Printf("Closing active connection and/or listeners")
	a.AbortActiveConnection(false)
	a.listenHub.Close()

	debug.Printf("Closing modems")
	if a.ardop != nil {
		if err := a.ardop.Close(); err != nil {
			log.Printf("Failure to close ardop TNC: %s", err)
		}
	}
	if a.pactor != nil {
		if err := a.pactor.Close(); err != nil {
			log.Printf("Failure to close pactor modem: %s", err)
		}
	}
	if a.varaFM != nil {
		if err := a.varaFM.Close(); err != nil {
			log.Printf("Failure to close varafm modem: %s", err)
		}
	}
	if a.varaHF != nil {
		if err := a.varaHF.Close(); err != nil {
			log.Printf("Failure to close varahf modem: %s", err)
		}
	}
	if a.agwpe != nil {
		if err := a.agwpe.Close(); err != nil {
			log.Printf("Failure to close AGWPE TNC: %s", err)
		}
	}

	// Close rigs
	debug.Printf("Closing rigs")
	for name, r := range a.rigs {
		if err := r.Close(); err != nil {
			log.Printf("Failure to close rig %s: %s", name, err)
		}
	}

	a.promptHub.Close()
	a.websocketHub.Close()
	a.eventLog.Close()
	a.formsMgr.Close()
}

func (a *App) loadHamlibRigs(rigsConfig map[string]cfg.HamlibConfig) {
	a.rigs = make(map[string]rig, len(rigsConfig))
	for name, conf := range rigsConfig {
		if conf.Address == "" {
			log.Printf("Missing address-field for rig '%s', skipping.", name)
			continue
		}
		if conf.Network == "" {
			conf.Network = "tcp"
		}

		r, err := hamlib.Open(conf.Network, conf.Address)
		if err != nil {
			log.Printf("Initialization hamlib rig %s failed: %s.", name, err)
			continue
		}

		var vfo hamlib.VFO
		switch strings.ToUpper(conf.VFO) {
		case "A", "VFOA":
			vfo, err = r.VFOA()
		case "B", "VFOB":
			vfo, err = r.VFOB()
		case "":
			vfo = r.CurrentVFO()
		default:
			log.Printf("Cannot load rig '%s': Unrecognized VFO identifier '%s'", name, conf.VFO)
			r.Close() // Close rig if we can't use it
			continue
		}

		if err != nil {
			log.Printf("Cannot load rig '%s': Unable to select VFO: %s", name, err)
			r.Close() // Close rig if we can't use it
			continue
		}

		f, err := vfo.GetFreq()
		if err != nil {
			log.Printf("Unable to get frequency from rig %s: %s.", name, err)
		} else {
			log.Printf("%s ready. Dial frequency is %s.", name, Frequency(f))
		}

		a.rigs[name] = rig{VFO: vfo, Closer: r}
	}
}
