// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
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

	"github.com/ogier/pflag"

	"github.com/la5nta/wl2k-go"
	"github.com/la5nta/wl2k-go/catalog"
	"github.com/la5nta/wl2k-go/mailbox"
	"github.com/la5nta/wl2k-go/rigcontrol/hamlib"
	"github.com/la5nta/wl2k-go/transport/winmor"

	"github.com/la5nta/wl2k-go/cmd/wl2k/cfg"
)

const (
	MethodWinmor    = "winmor"
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
	},
	{
		Str:  "interactive",
		Desc: "Run interactive mode.",
		HandleFunc: func(args []string) {
			Interactive()
		},
	},
	{
		Str:   "http",
		Desc:  "Run http server for web UI.",
		Usage: "[options]",
		Options: map[string]string{
			"--addr, -a": "Listen address. Default is :8080.",
		},
		HandleFunc: httpHandle,
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
		Str:        "position",
		Aliases:    []string{"pos"},
		Desc:       "Post a position report.",
		Usage:      "lat:lon[:comment]",
		HandleFunc: posReportHandle,
	},
	{
		Str:        "extract",
		Desc:       "Extract attachments from a message file.",
		Usage:      "file",
		HandleFunc: extractMessageHandle,
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
		Str:  "help",
		Desc: "Print detailed help for a given command.",
	},
}

var (
	config    cfg.Config
	rigs      map[string]*hamlib.Rig
	logWriter io.Writer

	exchangeChan chan ex                 // The channel that the exchange loop is listening on
	exchangeConn net.Conn                // Pointer to the active session connection (exchange)
	listeners    map[string]net.Listener // Active listeners
	mbox         *mailbox.DirHandler     // The mailbox
	wmTNC        *winmor.TNC             // Pointer to the WINMOR TNC used by Listen and Connect
)

var fOptions struct {
	IgnoreBusy bool // Move to connect?
	SendOnly   bool // Move to connect?

	MyCall      string
	Listen      string
	MailboxPath string
	ConfigPath  string
	LogPath     string
}

func optionsSet() *pflag.FlagSet {
	set := pflag.NewFlagSet("options", pflag.ExitOnError)

	defaultMBox, _ := mailbox.DefaultMailboxPath()

	set.StringVar(&fOptions.MyCall, `mycall`, ``, `Your callsign (winlink user).`)
	set.StringVarP(&fOptions.Listen, "listen", "l", "", "Comma-separated list of methods to listen on (e.g. winmor,telnet,ax25).")
	set.StringVar(&fOptions.MailboxPath, "mbox", defaultMBox, "Path to mailbox directory")
	set.StringVar(&fOptions.ConfigPath, "config", fOptions.ConfigPath, "Path to config file")
	set.StringVar(&fOptions.LogPath, "log", fOptions.LogPath, "Path to log file. The file is truncated on each startup.")
	set.BoolVarP(&fOptions.SendOnly, `send-only`, "s", false, `Download inbound messages later, send only.`)
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
	config, err := LoadConfig(fOptions.ConfigPath, cfg.DefaultConfig)
	if err != nil {
		log.Fatalf("Unable to load/write config: %s", err)
	}

	cmd, args := parseFlags(os.Args)

	// Skip initialization for some commands
	switch cmd.Str {
	case "help":
		helpHandle(args)
		return
	case "configure":
		cmd.HandleFunc(args)
		return
	}

	defer cleanup()

	// Logger
	f, err := os.Create(fOptions.LogPath)
	if err != nil {
		log.Fatal(err)
	}
	logWriter = io.MultiWriter(f, os.Stdout)
	log.SetOutput(logWriter)

	if fOptions.MyCall == "" && config.MyCall == "" {
		fmt.Fprint(os.Stderr, "Missing mycall\n")
		os.Exit(1)
	} else if fOptions.MyCall == "" {
		fOptions.MyCall = config.MyCall
	}

	if fOptions.Listen == "" && len(config.Listen) > 0 {
		fOptions.Listen = strings.Join(config.Listen, ",")
	}

	loadMBox()

	switch cmd.Str {
	case "connect", "http", "interactive":
		rigs = loadHamlibRigs()
		exchangeChan = exchangeLoop()
		scheduleLoop()
	}

	if fOptions.Listen != "" {
		Listen(fOptions.Listen)
	}

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
	set := pflag.NewFlagSet("http", pflag.ExitOnError)
	addr := set.StringP("addr", "a", ":8080", "Listen address.")
	set.Parse(args)

	if addr == nil {
		set.Usage()
		os.Exit(1)
	}

	if err := ListenAndServe(*addr); err != nil {
		log.Fatal(err)
	}
}

func connectHandle(args []string) {
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

	for _, rig := range rigs {
		rig.Close()
	}

	if wmTNC != nil {
		if err := wmTNC.Close(); err != nil {
			log.Fatalf("Failure to close winmor TNC: %s", err)
		}
	}
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

func loadHamlibRigs() map[string]*hamlib.Rig {
	rigs := make(map[string]*hamlib.Rig, len(config.HamlibRigs))

	for name, cfg := range config.HamlibRigs {
		rig, err := hamlib.Open(hamlib.RigModel(cfg.RigModel), cfg.TTYPath, cfg.Baudrate)
		if err != nil {
			log.Printf("Initialization hamlib rig %s failed: %s", name, err)
			continue
		}
		rigs[name] = rig
	}
	return rigs
}

func initWmTNC() {
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
		wmTNC.SetPTT(rig.CurrentVFO())
	}
}

func extractMessageHandle(args []string) {
	if len(args) == 0 || args[0] == "" {
		panic("TODO: usage")
	}

	file, _ := os.Open(args[0])
	defer file.Close()

	msg := new(wl2k.Message)
	if err := msg.ReadFrom(file); err != nil {
		log.Fatal(err)
	} else {
		fmt.Println(msg)
		for _, f := range msg.Files {
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

func composeMessage(replyMsg *wl2k.Message) {
	msg := wl2k.NewMessage(fOptions.MyCall)
	in := bufio.NewReader(os.Stdin)

	fmt.Printf(`From [%s]: `, fOptions.MyCall)
	from := readLine(in)
	if from == "" {
		from = fOptions.MyCall
	}
	msg.From = wl2k.AddressFromString(from)

	fmt.Print(`To`)
	if replyMsg != nil {
		fmt.Printf(" [%s]", replyMsg.From)

	}
	fmt.Printf(": ")
	to := readLine(in)
	if to == "" && replyMsg != nil {
		msg.To = append(msg.To, replyMsg.From)
	} else {
		for _, a := range strings.Split(to, `,`) {
			a = strings.TrimSpace(a)
			if a != "" {
				msg.To = append(msg.To, wl2k.AddressFromString(a))
			}
		}
	}

	ccCand := make([]wl2k.Address, 0)
	if replyMsg != nil {
		for _, addr := range append(replyMsg.To, replyMsg.Cc...) {
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
		msg.Cc = append(msg.Cc, ccCand...)
	} else {
		for _, a := range strings.Split(cc, `,`) {
			a = strings.TrimSpace(a)
			if a != "" {
				msg.Cc = append(msg.Cc, wl2k.AddressFromString(a))
			}
		}
	}

	if len(msg.To)+len(msg.Cc) == 1 {
		fmt.Print("P2P only [y/N]: ")
		ans := readLine(in)
		msg.P2POnly = (ans == "y" || ans == "Y")
	}

	fmt.Print(`Subject: `)
	if replyMsg != nil {
		subject := strings.TrimSpace(strings.TrimPrefix(replyMsg.Subject, "Re:"))
		subject = fmt.Sprintf("Re:%s", subject)
		fmt.Println(subject)
		msg.Subject = subject
	} else {
		msg.Subject = readLine(in)
	}

	// Read body

	fmt.Printf(`Press ENTER to start composing the message body. `)
	in.ReadString('\n')

	f, err := ioutil.TempFile("", fmt.Sprintf("wl2k_new_%d.txt", time.Now().Unix()))
	if err != nil {
		log.Fatalf("Unable to prepare temporary file for body: %s", err)
	}

	if replyMsg != nil {
		fmt.Fprintf(f, "--- %s %s wrote: ---\n", replyMsg.Date, replyMsg.From)
		orig := ">" + strings.Replace(
			strings.TrimSpace(string(replyMsg.Body)),
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
	msg.Body = wl2k.Body(buf.String())
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
			msg.AddFile(wl2k.NewFile(name, data))
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
	if len(args) == 0 {
		panic("TODO: Usage")
	}

	parts := strings.Split(args[0], ":")
	if len(parts) != 3 {
		log.Fatal(`Invalid position format. Expected "lat:lon:comment".`)
	}

	lat, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		log.Fatal(err)
	}

	lon, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		log.Fatal(err)
	}

	msg := catalog.PosReport{
		Lat:     &lat,
		Lon:     &lon,
		Date:    time.Now(),
		Comment: parts[2],
	}.Message(fOptions.MyCall)

	postMessage(msg)
}

func postMessage(msg *wl2k.Message) {
	if err := mbox.AddOut(msg); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Message posted")
}
