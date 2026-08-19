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
	"sync"
	"time"

	"github.com/harenber/ptc-go/v2/pactor"
	"github.com/la5nta/pat/api/types"
	"github.com/la5nta/pat/cfg"
	"github.com/la5nta/pat/internal/buildinfo"
	"github.com/la5nta/pat/internal/debug"
	"github.com/la5nta/pat/internal/directories"
	"github.com/la5nta/pat/internal/forms"
	"github.com/la5nta/pat/internal/propagation"
	"github.com/la5nta/pat/internal/varanny"

	"github.com/grandcat/zeroconf"
	"github.com/la5nta/wl2k-go/fbb"
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
	OnReload func() error

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

	predictor propagation.Predictor

	eventLog   *EventLogger
	termWriter io.WriteCloser // termWriter writes to both stdout and the log file (web gui echoes log file)

	// Varanny state
	varannyModems     map[string]varanny.ModemInfo // Discovered modems (name -> info)
	varannyModemsMu   sync.RWMutex
	varannySessions   map[string]*varanny.Session  // Active sessions keyed by scheme
	varannySessionsMu sync.RWMutex
	varannyCtx        context.Context               // Context for varanny operations
	varannyCancel     context.CancelFunc            // Cancel varanny discovery loop
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

	return &App{
		options:         opts,
		websocketHub:    noopWSSocket{},
		varannyModems:   make(map[string]varanny.ModemInfo),
		varannySessions: make(map[string]*varanny.Session),
	}
}

func (a *App) Mailbox() *mailbox.DirHandler { return a.mbox }

func (a *App) FormsManager() *forms.Manager { return a.formsMgr }

func (a *App) Config() cfg.Config { return a.config }

func (a *App) Options() Options { return a.options }

func (a *App) PromptHub() *PromptHub { return a.promptHub }

func (a *App) Reload() error { return a.OnReload() }

func (a *App) VFOForRig(rig string) (hamlib.VFO, bool) { r, ok := a.rigs[rig]; return r, ok }

// Locator implements forms.LocatorProvider
func (a *App) Locator() string { return a.config.Locator }

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

	// Check if we should use varanny CAT for VARA transports
	if (transport == MethodVaraHF || transport == MethodVaraFM) && a.config.Varanny.Enable && a.config.Varanny.UseVarannyCAT {
		modemType := "hf"
		if transport == MethodVaraFM {
			modemType = "fm"
		}

		a.varannyModemsMu.RLock()
		var modemInfo varanny.ModemInfo
		var found bool
		for _, m := range a.varannyModems {
			// Normalize type by removing trailing semicolon and whitespace
			normalizedType := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(m.Type, ";")))
			if normalizedType == modemType {
				modemInfo = m
				found = true
				log.Printf("Using varanny CAT for QSY: %s:%d", modemInfo.Host, modemInfo.CatPort)
				break
			}
		}
		a.varannyModemsMu.RUnlock()

		// If not found in cache, try on-demand discovery
		if !found {
			log.Printf("VFOForTransport: No matching modem in cache, trying on-demand discovery")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			modems, err := varanny.DiscoverModems(ctx)
			if err != nil {
				log.Printf("VFOForTransport: On-demand discovery failed: %v", err)
			} else {
				log.Printf("VFOForTransport: On-demand discovery found %d modems", len(modems))
				for _, m := range modems {
					normalizedType := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(m.Type, ";")))
					if normalizedType == modemType {
						modemInfo = m
						found = true
						log.Printf("VFOForTransport: Found modem via on-demand discovery: %s at %s:%d (CAT port: %d, dialect: %s)", modemInfo.Name, modemInfo.Host, modemInfo.CmdPort, modemInfo.CatPort, modemInfo.CatDialect)
						break
					}
				}
			}
		}

		if found && modemInfo.CatPort != 0 && modemInfo.CatDialect == "hamlib" {
			hamlibRig, err := hamlib.Open("tcp", fmt.Sprintf("%s:%d", modemInfo.Host, modemInfo.CatPort))
			if err == nil {
				log.Printf("Using varanny CAT for QSY: %s:%d", modemInfo.Host, modemInfo.CatPort)
				vfo := hamlibRig.CurrentVFO()
				return vfo, fmt.Sprintf("varanny:%s", modemInfo.Name), true, nil
			} else {
				log.Printf("Failed to connect to varanny CAT at %s:%d: %v", modemInfo.Host, modemInfo.CatPort, err)
			}
		} else if found {
			log.Printf("Varanny modem found but CAT not available: CatPort=%d, CatDialect=%s", modemInfo.CatPort, modemInfo.CatDialect)
		} else {
			log.Printf("No matching varanny modem found for type=%s", modemType)
		}
	}

	if rig == "" {
		return vfo, "", false, fmt.Errorf("missing rig reference in config section for %s", transport)
	}

	vfo, ok = a.VFOForRig(rig)
	if ok {
		return vfo, rig, ok, nil
	}

	return vfo, "", false, fmt.Errorf("hamlib rig '%s' not loaded", rig)
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
	a.termWriter = struct {
		io.Writer
		io.Closer
	}{io.MultiWriter(f, os.Stdout), f}
	log.SetOutput(io.MultiWriter(f, os.Stderr)) // web gui echoes the log file

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

	if a.options.Listen == "" && len(a.config.Listen) > 0 {
		a.options.Listen = strings.Join(a.config.Listen, ",")
	}

	// Initialize HF prediction engine (VOACAP)
	switch p := a.config.Prediction; p.Engine {
	case cfg.PredictionEngineVOACAP, cfg.PredictionEngineAuto:
		voacap, err := propagation.NewVOACAPPredictor(p.VOACAP.Executable, p.VOACAP.DataDir)
		if err != nil {
			// Only log error if engine is set explicitly
			if p.Engine == cfg.PredictionEngineVOACAP {
				log.Println("Failed to initialize VOACAP:", err)
			} else {
				debug.Printf("Failed to initialize VOACAP: %v", err)
			}
			break
		}
		debug.Printf("Prediction engine: %s (%q)", p.Engine, voacap.Version())
		a.predictor = propagation.WithCaching(voacap)
	case cfg.PredictionEngineVOACAPAPI:
		voacap := propagation.NewVOACAPAPIPredictor(p.VOACAPAPI.BaseURL)
		debug.Printf("Prediction engine: %s (%q)", p.Engine, voacap.Version())
		a.predictor = propagation.WithCaching(voacap)
	}

	// init forms subsystem
	a.formsMgr = forms.NewManager(forms.Config{
		FormsPath:       a.options.FormsPath,
		SequencePath:    filepath.Join(directories.StateDir(), "template-sequence-number.json"),
		SequenceFormat:  "%03d",
		MyCall:          a.options.MyCall,
		AppVersion:      buildinfo.AppName + " " + buildinfo.VersionStringShort(),
		UserAgent:       buildinfo.UserAgent(),
		GPSd:            a.config.GPSd,
		LocatorProvider: a,
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

	// Setup varanny context
	a.varannyCtx, a.varannyCancel = context.WithCancel(context.Background())

	// Start varanny discovery if enabled
	if a.config.Varanny.Enable {
		if a.config.Varanny.ContinuousDiscovery {
			a.startVarannyContinuousDiscovery()
		} else {
			a.refreshVarannyModems()
			go a.varannyDiscoveryLoop()
		}
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
		if a.config.GPSd.UpdateLocator {
			go a.gpsdLocatorUpdater(ctx)
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

	if ax25, err := ax25.Heard(a.Config().AX25Linux.Port); err == nil {
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
		if a.termWriter != nil {
			a.termWriter.Close()
		}
	}()

	debug.Printf("Closing active connection and/or listeners")
	a.AbortActiveConnection(false)
	a.listenHub.Close()

	// Cancel varanny discovery
	if a.varannyCancel != nil {
		a.varannyCancel()
	}

	// Clean up all varanny sessions
	// Collect schemes first to avoid holding lock during cleanup
	var schemes []string
	a.varannySessionsMu.Lock()
	for scheme := range a.varannySessions {
		schemes = append(schemes, scheme)
	}
	a.varannySessionsMu.Unlock()

	for _, scheme := range schemes {
		a.removeVarannySession(scheme)
	}

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

func (a *App) onServiceMessageReceived(msg *fbb.Message) {
	// Recover any panic here, as we really REALLY don't want the user to lose system messages due to panics.
	defer func() {
		if r := recover(); r != nil {
			log.Println(r)
		}
	}()

	// Write all service messages to the log
	body, _ := msg.Body()
	subject := msg.Subject()
	fmt.Fprintln(a.termWriter)
	fmt.Fprintln(a.termWriter, strings.Repeat("=", (60-len(subject))/2), subject, strings.Repeat("=", (60-len(subject))/2))
	fmt.Fprintln(a.termWriter, strings.TrimSpace(body))
	fmt.Fprintln(a.termWriter, strings.Repeat("=", 62))
	fmt.Fprintln(a.termWriter)

	// Handle account activation email
	if isActivation, password := isAccountActivationMessage(msg); isActivation && password != "" {
		fmt.Fprintln(a.termWriter, "DO NOT LOSE YOUR PASSWORD:", password)
		fmt.Fprintln(a.termWriter)
	}
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

func (a *App) SetConnectAliases(aliases map[string]string) error {
	onDisk, err := LoadConfig(a.options.ConfigPath, cfg.DefaultConfig)
	if err != nil {
		return err
	}
	onDisk.ConnectAliases = aliases
	if err := WriteConfig(onDisk, a.options.ConfigPath); err != nil {
		return err
	}
	a.config.ConnectAliases = aliases
	return nil
}

// getVarannySession returns a varanny session by scheme.
func (a *App) getVarannySession(scheme string) (*varanny.Session, bool) {
	a.varannySessionsMu.RLock()
	defer a.varannySessionsMu.RUnlock()
	session, ok := a.varannySessions[scheme]
	return session, ok
}

// setVarannySession sets a varanny session by scheme.
func (a *App) setVarannySession(scheme string, session *varanny.Session) {
	a.varannySessionsMu.Lock()
	defer a.varannySessionsMu.Unlock()
	a.varannySessions[scheme] = session
}

// removeVarannySession removes a varanny session by scheme.
func (a *App) removeVarannySession(scheme string) {
	a.varannySessionsMu.Lock()
	session, ok := a.varannySessions[scheme]
	if !ok {
		a.varannySessionsMu.Unlock()
		return
	}
	delete(a.varannySessions, scheme)
	a.varannySessionsMu.Unlock()

	// Close session outside the lock to avoid holding it during I/O
	if session != nil {
		if err := session.Close(); err != nil {
			log.Printf("Error closing varanny session for %s: %v", scheme, err)
		}
	}
}

// GetVarannyModems returns a copy of the discovered varanny modems.
func (a *App) GetVarannyModems() []varanny.ModemInfo {
	a.varannyModemsMu.RLock()
	defer a.varannyModemsMu.RUnlock()

	modems := make([]varanny.ModemInfo, 0, len(a.varannyModems))
	for _, m := range a.varannyModems {
		modems = append(modems, m)
	}
	return modems
}

// GetVarannySessionCount returns the number of active varanny sessions.
func (a *App) GetVarannySessionCount() int {
	a.varannySessionsMu.RLock()
	defer a.varannySessionsMu.RUnlock()
	return len(a.varannySessions)
}

// startVarannyContinuousDiscovery runs continuous event-driven mDNS discovery.
func (a *App) startVarannyContinuousDiscovery() {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		log.Printf("Failed to create varanny zeroconf resolver: %v", err)
		return
	}

	entries := make(chan *zeroconf.ServiceEntry)
	errChan := make(chan error, 1)

	go func() {
		errChan <- resolver.Browse(a.varannyCtx, "_vara-modem._tcp", "local.", entries)
	}()

	go func() {
		defer close(entries)

		for {
			select {
			case entry, ok := <-entries:
				if !ok {
					if err := <-errChan; err != nil {
						log.Printf("Varanny discovery error: %v", err)
					}
					return
				}

				info, err := varanny.ParseZeroconfEntry(entry)
				if err != nil {
					log.Printf("Failed to parse varanny entry: %v", err)
					continue
				}

				a.varannyModemsMu.Lock()
				info.FirstSeen = time.Now()
				info.LastSeen = time.Now()
				existing, existed := a.varannyModems[info.Name]
				if existed {
					info.FirstSeen = existing.FirstSeen
				}
				a.varannyModems[info.Name] = info
				a.varannyModemsMu.Unlock()

				if !existed {
					log.Printf("Discovered varanny modem: %s (%s) at %s:%d",
						info.Name, info.Type, info.Host, info.CmdPort)
				}

			case <-a.varannyCtx.Done():
				return
			}
		}
	}()
}

// refreshVarannyModems discovers varanny modems via mDNS (one-shot).
func (a *App) refreshVarannyModems() error {
	if !a.config.Varanny.Enable {
		return nil
	}

	ctx, cancel := context.WithTimeout(a.varannyCtx,
		time.Duration(a.config.Varanny.DiscoveryTimeout)*time.Second)
	defer cancel()

	modems, err := varanny.DiscoverModems(ctx)
	if err != nil {
		log.Printf("Varanny discovery failed: %v", err)
		return err
	}

	if len(modems) == 0 {
		log.Printf("No varanny modems found")
		return nil
	}

	a.varannyModemsMu.Lock()
	defer a.varannyModemsMu.Unlock()

	now := time.Now()
	for _, m := range modems {
		existing, existed := a.varannyModems[m.Name]
		if existed {
			m.FirstSeen = existing.FirstSeen
		} else {
			m.FirstSeen = now
		}
		m.LastSeen = now
		a.varannyModems[m.Name] = m
		if !existed {
			log.Printf("Discovered varanny modem: %s (%s) at %s:%d",
				m.Name, m.Type, m.Host, m.CmdPort)
		}
	}

	return nil
}

// pruneExpiredModems removes modems that haven't been seen within TTL.
func (a *App) pruneExpiredModems() {
	ttl := time.Duration(a.config.Varanny.ModemTTL) * time.Minute

	var removed []string
	a.varannyModemsMu.Lock()
	for name, modem := range a.varannyModems {
		if modem.IsExpired(ttl) {
			removed = append(removed, name)
			// Safe to delete immediately - fast CPU-bound operation
			delete(a.varannyModems, name)
		}
	}
	a.varannyModemsMu.Unlock()

	// Log outside the lock to avoid I/O while holding mutex
	for _, name := range removed {
		log.Printf("Pruned expired varanny modem: %s", name)
	}
}

// varannyDiscoveryLoop periodically refreshes varanny modem discovery.
func (a *App) varannyDiscoveryLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	pruneTicker := time.NewTicker(2 * time.Minute)
	defer pruneTicker.Stop()

	for {
		select {
		case <-ticker.C:
			a.refreshVarannyModems()
		case <-pruneTicker.C:
			a.pruneExpiredModems()
		case <-a.varannyCtx.Done():
			return
		}
	}
}

// findVarannyModem finds a varanny modem by type.
func (a *App) findVarannyModem(modemType string) (varanny.ModemInfo, bool) {
	a.varannyModemsMu.RLock()
	defer a.varannyModemsMu.RUnlock()

	// Check for preferred modem based on type
	preferred := ""
	if modemType == "hf" {
		preferred = a.config.Varanny.PreferredHFModem
	} else if modemType == "fm" {
		preferred = a.config.Varanny.PreferredFMModem
	}

	if preferred != "" {
		if m, ok := a.varannyModems[preferred]; ok && m.Type == modemType {
			return m, true
		}
	}

	// Find first matching type
	for _, m := range a.varannyModems {
		if m.Type == modemType {
			return m, true
		}
	}

	return varanny.ModemInfo{}, false
}
