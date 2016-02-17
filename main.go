// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// A fully working Winlink 2000 command line client with support for various connection methods.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"

	"github.com/la5nta/wl2k-go/catalog"
	"github.com/la5nta/wl2k-go/fbb"
	"github.com/la5nta/wl2k-go/mailbox"
	"github.com/la5nta/wl2k-go/rigcontrol/hamlib"
	"github.com/la5nta/wl2k-go/transport/ardop"
	"github.com/la5nta/wl2k-go/transport/winmor"

	"github.com/la5nta/wl2k-go/cmd/wl2k/cfg"
	"github.com/la5nta/wl2k-go/cmd/wl2k/internal/gpsd"
)

const (
	MethodWinmor    = "winmor"
	MethodArdop     = "ardop"
	MethodTelnet    = "telnet"
	MethodAX25      = "ax25"
	MethodSerialTNC = "serial-tnc"
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
		Str:  "interactive",
		Desc: "Run interactive mode.",
		HandleFunc: func(args []string) {
			Interactive()
		},
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
		Str:     "position",
		Aliases: []string{"pos"},
		Desc:    "Post a position report (GPSd or manual entry).",
		Usage:   "[options]",
		Options: map[string]string{
			"--latlon":      "latitude,longitude for manual entry. Will use GPSd if this is empty.",
			"--comment, -c": "Comment to be included in the position report.",
		},
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
			"--force-download, -d": "Force download of latest list from winlink.org.",
		},
		HandleFunc: rmsListHandle,
	},
	{
		Str:        "riglist",
		Usage:      "[search term]",
		Desc:       "Print/search a list of rigcontrol supported transceivers.",
		HandleFunc: riglistHandle,
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
			fmt.Printf("wl2k v%s (%s) %s/%s\n", Version, GitRev, runtime.GOOS, runtime.GOARCH)
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

	exchangeChan chan ex                 // The channel that the exchange loop is listening on
	exchangeConn net.Conn                // Pointer to the active session connection (exchange)
	listeners    map[string]net.Listener // Active listeners
	mbox         *mailbox.DirHandler     // The mailbox
	wmTNC        *winmor.TNC             // Pointer to the WINMOR TNC used by Listen and Connect
	adTNC        *ardop.TNC              // Pointer to the ARDOP TNC used by Listen and Connect
)

var fOptions struct {
	IgnoreBusy bool // Move to connect?
	SendOnly   bool // Move to connect?

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
	set.BoolVarP(&fOptions.Robust, `robust`, "r", false, `Use robust modes only. (Usefull to improve s/n-ratio at remote winmor station)`)
	set.BoolVar(&fOptions.IgnoreBusy, "ignore-busy", false, "Don't wait for clear channel before connecting to a node.")

	return set
}

func init() {
	listeners = make(map[string]net.Listener)

	if appDir, err := mailbox.DefaultAppDir(); err != nil {
		log.Fatal(err)
	} else {
		fOptions.ConfigPath = path.Join(appDir, "config.json")
		fOptions.LogPath = path.Join(appDir, "wl2k.log")
		fOptions.EventLogPath = path.Join(appDir, "eventlog.json")
	}

	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s is a client for the Winlink 2000 Network.\n\n", os.Args[0])
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
	var err error
	config, err = LoadConfig(fOptions.ConfigPath, cfg.DefaultConfig)
	if err != nil {
		log.Fatalf("Unable to load/write config: %s", err)
	}

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

	// Don't use config password if we don't use config mycall
	if fOptions.MyCall != config.MyCall {
		config.SecureLoginPassword = ""
	}

	// Replace placeholders in connect aliases
	for k, v := range config.ConnectAliases {
		config.ConnectAliases[k] = strings.Replace(v, cfg.PlaceholderMycall, fOptions.MyCall, -1)
	}

	if fOptions.Listen == "" && len(config.Listen) > 0 {
		fOptions.Listen = strings.Join(config.Listen, ",")
	}

	// Make sure we clean up on exit, closing any open resources etc.
	defer cleanup()

	// Load the mailbox handler
	loadMBox()

	if cmd.MayConnect {
		rigs = loadHamlibRigs()
		exchangeChan = exchangeLoop()
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
	cmd := exec.Command(EditorName(), fOptions.ConfigPath)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("Unable to start editor: %s", err)
	}
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

func riglistHandle(args []string) {
	term := strings.ToLower(args[0])

	fmt.Print("id\ttransceiver\n")
	for m, str := range hamlib.Rigs() {
		if !strings.Contains(strings.ToLower(str), term) {
			continue
		}
		fmt.Printf("%d\t%s\n", m, str)
	}
}

func cleanup() {
	for method, _ := range listeners {
		Unlisten(method)
	}

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

func initWinmorTNC() {
	var err error
	wmTNC, err = winmor.Open(config.Winmor.Addr, fOptions.MyCall, config.Locator)
	if err != nil {
		log.Fatalf("WINMOR TNC initialization failed: %s", err)
	}

	if v, err := wmTNC.Version(); err != nil {
		log.Fatalf("WINMOR TNC initialization failed: %s", err)
	} else {
		log.Printf("WINMOR TNC v%s initialized", v)
	}

	if !config.Winmor.PTTControl {
		return
	}

	rig, ok := rigs[config.Winmor.Rig]
	if !ok {
		log.Printf("Unable to set PTT rig '%s': Not defined or not loaded.", config.Winmor.Rig)
	} else {
		wmTNC.SetPTT(rig)
	}
}

func initArdopTNC() {
	var err error
	adTNC, err = ardop.OpenTCP(config.Ardop.Addr, fOptions.MyCall, config.Locator)
	if err != nil {
		log.Fatalf("ARDOP TNC initialization failed: %s", err)
	}

	if !config.Ardop.ARQBandwidth.IsZero() {
		if err := adTNC.SetARQBandwidth(config.Ardop.ARQBandwidth); err != nil {
			log.Fatalf("Unable to set ARQ bandwidth for ardop TNC: %s", err)
		}
	}

	if err := adTNC.SetCWID(config.Ardop.CWID); err != nil {
		log.Fatalf("Unable to configure CWID for ardop TNC: %s", err)
	}

	if v, err := adTNC.Version(); err != nil {
		log.Fatalf("ARDOP TNC initialization failed: %s", err)
	} else {
		log.Printf("ARDOP TNC (%s) initialized", v)
	}

	if !config.Ardop.PTTControl {
		return
	}

	rig, ok := rigs[config.Ardop.Rig]
	if !ok {
		log.Printf("Unable to set PTT rig '%s': Not defined or not loaded.", config.Ardop.Rig)
	} else {
		wmTNC.SetPTT(rig)
	}
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
			if err := ioutil.WriteFile(f.Name(), f.Data(), 0664); err != nil {
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

func composeMessage(replyMsg *fbb.Message) {
	msg := fbb.NewMessage(fbb.Private, fOptions.MyCall)
	in := bufio.NewReader(os.Stdin)

	fmt.Printf(`From [%s]: `, fOptions.MyCall)
	from := readLine(in)
	if from == "" {
		from = fOptions.MyCall
	}
	msg.SetFrom(from)

	fmt.Print(`To`)
	if replyMsg != nil {
		fmt.Printf(" [%s]", replyMsg.From())

	}
	fmt.Printf(": ")
	to := readLine(in)
	if to == "" && replyMsg != nil {
		msg.AddTo(replyMsg.From().String())
	} else {
		for _, addr := range strings.Split(to, `,`) {
			addr = strings.TrimSpace(addr)
			if addr != "" {
				msg.AddTo(addr)
			}
		}
	}

	ccCand := make([]fbb.Address, 0)
	if replyMsg != nil {
		for _, addr := range append(replyMsg.To(), replyMsg.Cc()...) {
			if !addr.EqualString(fOptions.MyCall) {
				ccCand = append(ccCand, addr)
			}
		}
	}

	fmt.Printf("Cc")
	if replyMsg != nil {
		fmt.Printf(" %s", ccCand)
	}
	fmt.Print(`: `)
	cc := readLine(in)
	if cc == "" && replyMsg != nil {
		for _, addr := range ccCand {
			msg.AddCc(addr.String())
		}
	} else {
		for _, addr := range strings.Split(cc, `,`) {
			addr = strings.TrimSpace(addr)
			if addr != "" {
				msg.AddCc(addr)
			}
		}
	}

	if len(msg.Receivers()) == 1 {
		fmt.Print("P2P only [y/N]: ")
		ans := readLine(in)
		if strings.EqualFold("y", ans) {
			msg.Header.Set("X-P2POnly", "true")
		}
	}

	fmt.Print(`Subject: `)
	if replyMsg != nil {
		subject := strings.TrimSpace(strings.TrimPrefix(replyMsg.Subject(), "Re:"))
		subject = fmt.Sprintf("Re:%s", subject)
		fmt.Println(subject)
		msg.SetSubject(subject)
	} else {
		msg.SetSubject(readLine(in))
	}

	// Read body

	fmt.Printf(`Press ENTER to start composing the message body. `)
	in.ReadString('\n')

	f, err := ioutil.TempFile("", fmt.Sprintf("wl2k_new_%d.txt", time.Now().Unix()))
	if err != nil {
		log.Fatalf("Unable to prepare temporary file for body: %s", err)
	}

	if replyMsg != nil {
		fmt.Fprintf(f, "--- %s %s wrote: ---\n", replyMsg.Date(), replyMsg.From().Addr)
		body, _ := replyMsg.Body()
		orig := ">" + strings.Replace(
			strings.TrimSpace(body),
			"\n",
			"\n>",
			-1,
		) + "\n"
		f.Write([]byte(orig))
		f.Sync()
	}

	cmd := exec.Command(EditorName(), f.Name())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("Unable to start body editor: %s", err)
	}

	f.Seek(0, 0)

	var buf bytes.Buffer
	io.Copy(&buf, f)
	msg.SetBody(buf.String())
	f.Close()
	os.Remove(f.Name())

	// END Read body

	fmt.Print("\n")

	for {
		fmt.Print(`Attachment [empty when done]: `)
		path := readLine(in)
		if path == "" {
			break
		}

		data, err := ioutil.ReadFile(path)
		if err != nil {
			log.Println(err)
		} else {
			_, name := filepath.Split(path)
			msg.AddFile(fbb.NewFile(name, data))
		}
	}
	fmt.Println(msg)
	postMessage(msg)
}

func readLine(r *bufio.Reader) string {
	str, _ := r.ReadString('\n')
	return strings.TrimSpace(str)
}

func posReportHandle(args []string) {
	var latlon, comment string

	set := pflag.NewFlagSet("position", pflag.ExitOnError)
	set.StringVar(&latlon, "latlon", "", "latitude,longitude for manual entry")
	set.StringVarP(&comment, "comment", "c", "", "Comment")
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
	} else if config.GPSdAddr != "" {
		conn, err := gpsd.Dial(config.GPSdAddr)
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
		report.Date = pos.Time

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
	if err := mbox.AddOut(msg); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Message posted")
}
