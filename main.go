// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// A portable Winlink client for amateur radio email.
package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"

	"github.com/la5nta/wl2k-go/catalog"
	"github.com/la5nta/wl2k-go/fbb"
	"github.com/la5nta/wl2k-go/mailbox"
	"github.com/la5nta/wl2k-go/rigcontrol/hamlib"

	"github.com/la5nta/pat/cfg"
	"github.com/la5nta/pat/internal/forms"
	"github.com/la5nta/pat/internal/gpsd"
)

const (
	MethodWinmor    = "winmor"
	MethodArdop     = "ardop"
	MethodTelnet    = "telnet"
	MethodAX25      = "ax25"
	MethodSerialTNC = "serial-tnc"
	MethodPactor    = "pactor"
)

var commands = []Command{
	{
		Str:        "connect",
		Desc:       "Connect to a remote station.",
		HandleFunc: connectHandle,
		Usage:      UsageConnect,
		Example:    ExampleConnect,
		MayConnect: true,
	},
	{
		Str:   "interactive",
		Desc:  "Run interactive mode.",
		Usage: "[options]",
		Options: map[string]string{
			"--http, -h": "Start http server for web UI in the background.",
		},
		HandleFunc: InteractiveHandle,
		MayConnect: true,
		LongLived:  true,
	},
	{
		Str:   "http",
		Desc:  "Run http server for web UI.",
		Usage: "[options]",
		Options: map[string]string{
			"--addr, -a": "Listen address. Default is :8080.",
		},
		HandleFunc: httpHandle,
		MayConnect: true,
		LongLived:  true,
	},
	{
		Str:  "compose",
		Desc: "Compose a new message.",
		HandleFunc: func(args []string) {
			composeMessage(nil)
		},
	},
	{
		Str:  "read",
		Desc: "Read messages.",
		HandleFunc: func(args []string) {
			readMail()
		},
	},
	{
		Str:     "composeform",
		Aliases: []string{"formPath"},
		Desc:    "Post form-based report.",
		Usage:   "[options]",
		Options: map[string]string{
			"--template": "path to the form template file. Uses the config file's forms_path as root. Defaults to 'ICS USA Forms/ICS213.txt'",
		},
		HandleFunc: composeFormReport,
	},
	{
		Str:     "position",
		Aliases: []string{"pos"},
		Desc:    "Post a position report (GPSd or manual entry).",
		Usage:   "[options]",
		Options: map[string]string{
			"--latlon":      "latitude,longitude in decimal degrees for manual entry. Will use GPSd if this is empty.",
			"--comment, -c": "Comment to be included in the position report.",
		},
		Example:    ExamplePosition,
		HandleFunc: posReportHandle,
	},
	{
		Str:        "extract",
		Desc:       "Extract attachments from a message file.",
		Usage:      "file",
		HandleFunc: extractMessageHandle,
	},
	{
		Str:   "rmslist",
		Desc:  "Print/search in list of RMS nodes.",
		Usage: "[options] [search term]",
		Options: map[string]string{
			"--mode, -m":           "Mode filter.",
			"--band, -b":           "Band filter (e.g. '80m').",
			"--force-download, -d": "Force download of latest list from winlink.org.",
			"--sort-distance, -s":  "Sort by distance",
		},
		HandleFunc: rmsListHandle,
	},
	{
		Str:  "updateforms",
		Desc: "Download the latest form templates from winlink.org.",
		HandleFunc: func(args []string) {
			_, _ = formsMgr.UpdateFormTemplates()
		},
	},
	{
		Str:        "configure",
		Desc:       "Open configuration file for editing.",
		HandleFunc: configureHandle,
	},
	{
		Str:  "version",
		Desc: "Print the application version",
		HandleFunc: func(args []string) {
			fmt.Printf("%s %s\n", AppName, versionString())
		},
	},
	{
		Str:  "help",
		Desc: "Print detailed help for a given command.",
		// Avoid initialization loop by invoking helpHandler in main
	},
}

var (
	config    cfg.Config
	rigs      map[string]hamlib.VFO
	logWriter io.Writer
	eventLog  *EventLogger

	exchangeChan chan ex             // The channel that the exchange loop is listening on
	exchangeConn net.Conn            // Pointer to the active session connection (exchange)
	mbox         *mailbox.DirHandler // The mailbox
	listenHub    *ListenerHub
	promptHub    *PromptHub
	formsMgr     *forms.Manager

	appDir string
)

var fOptions struct {
	IgnoreBusy bool // Move to connect?
	SendOnly   bool // Move to connect?
	RadioOnly  bool

	Robust       bool
	MyCall       string
	Listen       string
	MailboxPath  string
	ConfigPath   string
	LogPath      string
	EventLogPath string
}

func optionsSet() *pflag.FlagSet {
	set := pflag.NewFlagSet("options", pflag.ExitOnError)

	defaultMBox, _ := mailbox.DefaultMailboxPath()

	set.StringVar(&fOptions.MyCall, `mycall`, ``, `Your callsign (winlink user).`)
	set.StringVarP(&fOptions.Listen, "listen", "l", "", "Comma-separated list of methods to listen on (e.g. winmor,ardop,telnet,ax25).")
	set.StringVar(&fOptions.MailboxPath, "mbox", defaultMBox, "Path to mailbox directory")
	set.StringVar(&fOptions.ConfigPath, "config", fOptions.ConfigPath, "Path to config file")
	set.StringVar(&fOptions.LogPath, "log", fOptions.LogPath, "Path to log file. The file is truncated on each startup.")
	set.StringVar(&fOptions.EventLogPath, "event-log", fOptions.EventLogPath, "Path to event log file.")
	set.BoolVarP(&fOptions.SendOnly, `send-only`, "s", false, `Download inbound messages later, send only.`)
	set.BoolVarP(&fOptions.RadioOnly, `radio-only`, "", false, `Radio Only mode (Winlink Hybrid RMS only).`)
	set.BoolVarP(&fOptions.Robust, `robust`, "r", false, `Use robust modes only. (Useful to improve s/n-ratio at remote winmor station)`)
	set.BoolVar(&fOptions.IgnoreBusy, "ignore-busy", false, "Don't wait for clear channel before connecting to a node.")

	return set
}

func init() {
	listenHub = NewListenerHub()
	promptHub = NewPromptHub()

	var err error
	appDir, err = mailbox.DefaultAppDir()
	if err != nil {
		log.Fatal(err)
	}

	fOptions.ConfigPath = path.Join(appDir, "config.json")
	fOptions.LogPath = path.Join(appDir, strings.ToLower(AppName+".log"))
	fOptions.EventLogPath = path.Join(appDir, "eventlog.json")

	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s is a client for the Winlink 2000 Network.\n\n", AppName)
		fmt.Fprintf(os.Stderr, "Usage:\n  %s [options] command [arguments]\n", os.Args[0])

		fmt.Fprintln(os.Stderr, "\nCommands:")
		for _, cmd := range commands {
			fmt.Fprintf(os.Stderr, "  %-15s %s\n", cmd.Str, cmd.Desc)
		}

		fmt.Fprintln(os.Stderr, "\nOptions:")
		optionsSet().PrintDefaults()
		fmt.Fprint(os.Stderr, "\n")
	}
}

func main() {
	cmd, args := parseFlags(os.Args)

	// Skip initialization for some commands
	switch cmd.Str {
	case "help":
		helpHandle(args)
		return
	case "configure", "version":
		cmd.HandleFunc(args)
		return
	}

	// Enable the GZIP extension experiment by default
	if _, ok := os.LookupEnv("GZIP_EXPERIMENT"); !ok {
		os.Setenv("GZIP_EXPERIMENT", "1")
	}

	// Parse configuration file
	var err error
	config, err = LoadConfig(fOptions.ConfigPath, cfg.DefaultConfig)
	if err != nil {
		log.Fatalf("Unable to load/write config: %s", err)
	}

	// Initialize logger
	f, err := os.Create(fOptions.LogPath)
	if err != nil {
		log.Fatal(err)
	}
	logWriter = io.MultiWriter(f, os.Stdout)
	log.SetOutput(logWriter)
	eventLog, err = NewEventLogger(fOptions.EventLogPath)
	if err != nil {
		log.Fatal("Unable to open event log file:", err)
	}

	// Read command line options from config if unset
	if fOptions.MyCall == "" && config.MyCall == "" {
		fmt.Fprint(os.Stderr, "Missing mycall\n")
		os.Exit(1)
	} else if fOptions.MyCall == "" {
		fOptions.MyCall = config.MyCall
	}

	// Ensure mycall is all upper case.
	fOptions.MyCall = strings.ToUpper(fOptions.MyCall)

	// Don't use config password if we don't use config mycall
	if !strings.EqualFold(fOptions.MyCall, config.MyCall) {
		config.SecureLoginPassword = ""
	}

	// Replace placeholders in connect aliases
	for k, v := range config.ConnectAliases {
		config.ConnectAliases[k] = strings.Replace(v, cfg.PlaceholderMycall, fOptions.MyCall, -1)
	}

	if fOptions.Listen == "" && len(config.Listen) > 0 {
		fOptions.Listen = strings.Join(config.Listen, ",")
	}

	// init forms subsystem
	formsMgr = forms.NewManager(forms.Config{
		FormsPath:  config.FormsPath,
		MyCall:     fOptions.MyCall,
		Locator:    config.Locator,
		AppVersion: versionStringShort(),
		UserAgent:  PatUserAgent,
		LineReader: readLine,
	})

	// Make sure we clean up on exit, closing any open resources etc.
	defer cleanup()

	// Load the mailbox handler
	loadMBox()

	if cmd.MayConnect {
		rigs = loadHamlibRigs()
		exchangeChan = exchangeLoop()

		go func() {
			if config.VersionReportingDisabled {
				return
			}

			for { // Check every 6 hours, but it won't post more frequent than 24h.
				postVersionUpdate() // Ignore errors
				time.Sleep(6 * time.Hour)
			}
		}()
	}

	if cmd.LongLived {
		if fOptions.Listen != "" {
			Listen(fOptions.Listen)
		}
		scheduleLoop()
	}

	// Start command execution
	cmd.HandleFunc(args)
}

func configureHandle(args []string) {
	// Ensure config file has been written
	_, err := ReadConfig(fOptions.ConfigPath)
	if os.IsNotExist(err) {
		err = WriteConfig(cfg.DefaultConfig, fOptions.ConfigPath)
		if err != nil {
			log.Fatalf("Unable to write default config: %s", err)
		}
	}

	cmd := exec.Command(EditorName(), fOptions.ConfigPath)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("Unable to start editor: %s", err)
	}
}

func InteractiveHandle(args []string) {
	var http string
	set := pflag.NewFlagSet("interactive", pflag.ExitOnError)
	set.StringVar(&http, "http", "", "HTTP listen address")
	set.Lookup("http").NoOptDefVal = config.HTTPAddr
	set.Parse(args)

	if http == "" {
		Interactive()
		return
	}

	go func() {
		if err := ListenAndServe(http); err != nil {
			log.Println(err)
		}
	}()
	time.Sleep(time.Second)
	Interactive()
}

func httpHandle(args []string) {
	addr := config.HTTPAddr
	if addr == "" {
		addr = ":8080" // For backwards compatibility (remove in future)
	}

	set := pflag.NewFlagSet("http", pflag.ExitOnError)
	set.StringVarP(&addr, "addr", "a", addr, "Listen address.")
	set.Parse(args)

	if addr == "" {
		set.Usage()
		os.Exit(1)
	}

	promptHub.OmitTerminal(true)

	if err := ListenAndServe(addr); err != nil {
		log.Fatal(err)
	}
}

func connectHandle(args []string) {
	if args[0] == "" {
		fmt.Println("Missing argument, try 'connect help'.")
	}
	if success := Connect(args[0]); !success {
		os.Exit(1)
	}
}

func helpHandle(args []string) {
	arg := args[0]

	var cmd *Command
	for _, c := range commands {
		if c.Str == arg {
			cmd = &c
			break
		}
	}
	if arg == "" || cmd == nil {
		pflag.Usage()
		return
	}
	cmd.PrintUsage()
}

func cleanup() {
	listenHub.Close()

	if wmTNC != nil {
		if err := wmTNC.Close(); err != nil {
			log.Fatalf("Failure to close winmor TNC: %s", err)
		}
	}

	if adTNC != nil {
		if err := adTNC.Close(); err != nil {
			log.Fatalf("Failure to close ardop TNC: %s", err)
		}
	}

	if pModem != nil {
		if err := pModem.Close(); err != nil {
			log.Fatalf("Failure to close pactor modem: %s", err)
		}
	}

	eventLog.Close()
}

func loadMBox() {
	mbox = mailbox.NewDirHandler(
		path.Join(fOptions.MailboxPath, fOptions.MyCall),
		fOptions.SendOnly,
	)

	// Ensure the mailbox handler is ready
	if err := mbox.Prepare(); err != nil {
		log.Fatal(err)
	}
}

func loadHamlibRigs() map[string]hamlib.VFO {
	rigs := make(map[string]hamlib.VFO, len(config.HamlibRigs))

	for name, cfg := range config.HamlibRigs {
		if cfg.Address == "" {
			log.Printf("Missing address-field for rig '%s', skipping.", name)
			continue
		}

		rig, err := hamlib.Open(cfg.Network, cfg.Address)
		if err != nil {
			log.Printf("Initialization hamlib rig %s failed: %s.", name, err)
			continue
		}

		var vfo hamlib.VFO
		switch strings.ToUpper(cfg.VFO) {
		case "A", "VFOA":
			vfo, err = rig.VFOA()
		case "B", "VFOB":
			vfo, err = rig.VFOB()
		case "":
			vfo = rig.CurrentVFO()
		default:
			log.Printf("Cannot load rig '%s': Unrecognized VFO identifier '%s'", name, cfg.VFO)
			continue
		}

		if err != nil {
			log.Printf("Cannot load rig '%s': Unable to select VFO: %s", name, err)
			continue
		}

		f, err := vfo.GetFreq()
		if err != nil {
			log.Printf("Unable to get frequency from rig %s: %s.", name, err)
		} else {
			log.Printf("%s ready. Dial frequency is %s.", name, Frequency(f))
		}

		rigs[name] = vfo
	}
	return rigs
}

func extractMessageHandle(args []string) {
	if len(args) == 0 || args[0] == "" {
		panic("TODO: usage")
	}

	file, _ := os.Open(args[0])
	defer file.Close()

	msg := new(fbb.Message)
	if err := msg.ReadFrom(file); err != nil {
		log.Fatal(err)
	} else {
		fmt.Println(msg)
		for _, f := range msg.Files() {
			if err := os.WriteFile(f.Name(), f.Data(), 0664); err != nil {
				log.Fatal(err)
			}
		}
	}
}

func EditorName() string {
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	} else if e := os.Getenv("VISUAL"); e != "" {
		return e
	}

	switch runtime.GOOS {
	case "windows":
		return "notepad"
	case "linux":
		if path, err := exec.LookPath("editor"); err == nil {
			return path
		}
	}

	return "vi"
}

func posReportHandle(args []string) {
	var latlon, comment string

	set := pflag.NewFlagSet("position", pflag.ExitOnError)
	set.StringVar(&latlon, "latlon", "", "")
	set.StringVarP(&comment, "comment", "c", "", "")
	set.Parse(args)

	report := catalog.PosReport{Comment: comment}

	if latlon != "" {
		parts := strings.Split(latlon, ",")
		if len(parts) != 2 {
			log.Fatal(`Invalid position format. Expected "latitude,longitude".`)
		}

		lat, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			log.Fatal(err)
		}
		report.Lat = &lat

		lon, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			log.Fatal(err)
		}
		report.Lon = &lon
	} else if config.GPSd.Addr != "" {
		conn, err := gpsd.Dial(config.GPSd.Addr)
		if err != nil {
			log.Fatalf("GPSd daemon: %s", err)
		}
		defer conn.Close()

		conn.Watch(true)

		log.Println("Waiting for position from GPSd...") //TODO: Spinning bar?
		pos, err := conn.NextPos()
		if err != nil {
			log.Fatalf("GPSd: %s", err)
		}

		report.Lat = &pos.Lat
		report.Lon = &pos.Lon
		if config.GPSd.UseServerTime {
			report.Date = time.Now()
		} else {
			report.Date = pos.Time
		}

		// Course and speed is part of the spec, but does not seem to be
		// supported by winlink.org anymore. Ignore it for now.
		if false && pos.Track != 0 {
			course := CourseFromFloat64(pos.Track, false)
			report.Course = &course
		}
	} else {
		fmt.Println("No position available. See --help")
		os.Exit(1)
	}

	if report.Date.IsZero() {
		report.Date = time.Now()
	}

	postMessage(report.Message(fOptions.MyCall))
}

func CourseFromFloat64(f float64, magnetic bool) catalog.Course {
	c := catalog.Course{Magnetic: magnetic}

	str := fmt.Sprintf("%03.0f", f)
	for i := 0; i < 3; i++ {
		c.Digits[i] = str[i]
	}

	return c
}

func postMessage(msg *fbb.Message) {
	if err := msg.Validate(); err != nil {
		fmt.Printf("WARNING - Message does not validate: %s\n", err)
	}
	if err := mbox.AddOut(msg); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Message posted")
}

func versionString() string {
	return fmt.Sprintf("%s %s/%s - %s", versionStringShort(), runtime.GOOS, runtime.GOARCH, runtime.Version())
}

func versionStringShort() string {
	return fmt.Sprintf("v%s (%s)", Version, GitRev)
}
