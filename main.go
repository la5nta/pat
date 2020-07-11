// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// A portable Winlink client for amateur radio email.
package main

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
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

type MessageForm struct {
	Subject string
	Body string
	AttachmentXml string
}

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

func composeMessageHeader(replyMsg *fbb.Message) *fbb.Message {

	msg := fbb.NewMessage(fbb.Private, fOptions.MyCall)

	fmt.Printf(`From [%s]: `, fOptions.MyCall)
	from := readLine()
	if from == "" {
		from = fOptions.MyCall
	}
	msg.SetFrom(from)

	fmt.Print(`To`)
	if replyMsg != nil {
		fmt.Printf(" [%s]", replyMsg.From())

	}
	fmt.Printf(": ")
	to := readLine()
	if to == "" && replyMsg != nil {
		msg.AddTo(replyMsg.From().String())
	} else {
		for _, addr := range strings.FieldsFunc(to, SplitFunc) {
			msg.AddTo(addr)
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
	cc := readLine()
	if cc == "" && replyMsg != nil {
		for _, addr := range ccCand {
			msg.AddCc(addr.String())
		}
	} else {
		for _, addr := range strings.FieldsFunc(cc, SplitFunc) {
			msg.AddCc(addr)
		}
	}

	switch len(msg.Receivers()) {
	case 1:
		fmt.Print("P2P only [y/N]: ")
		ans := readLine()
		if strings.EqualFold("y", ans) {
			msg.Header.Set("X-P2POnly", "true")
		}
	case 0:
		fmt.Println("Message must have at least one recipient")
		os.Exit(1)
	}

	fmt.Print(`Subject: `)
	if replyMsg != nil {
		subject := strings.TrimSpace(strings.TrimPrefix(replyMsg.Subject(), "Re:"))
		subject = fmt.Sprintf("Re:%s", subject)
		fmt.Println(subject)
		msg.SetSubject(subject)
	} else {
		msg.SetSubject(readLine())
	}
	// A message without subject is not valid, so let's use a sane default
	if msg.Subject() == "" {
		msg.SetSubject("<No subject>")
	}

	return msg

}

func composeMessage(replyMsg *fbb.Message) {

	msg := composeMessageHeader(replyMsg)

	// Read body
	fmt.Printf(`Press ENTER to start composing the message body. `)
	readLine()

	f, err := ioutil.TempFile("", strings.ToLower(fmt.Sprintf("%s_new_%d.txt", AppName, time.Now().Unix())))
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

	// Windows fix: Avoid 'cannot access the file because it is being used by another process' error.
	// Close the file before opening the editor.
	f.Close()

	cmd := exec.Command(EditorName(), f.Name())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("Unable to start body editor: %s", err)
	}

	f, err = os.OpenFile(f.Name(), os.O_RDWR, 0666)
	if err != nil {
		log.Fatalf("Unable to read temporary file from editor: %s", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, f)
	msg.SetBody(buf.String())
	f.Close()
	os.Remove(f.Name())

	// An empty message body is illegal. Let's set a sane default.
	if msg.BodySize() == 0 {
		msg.SetBody("<No message body>\n")
	}

	// END Read body

	fmt.Print("\n")

	for {
		fmt.Print(`Attachment [empty when done]: `)
		path := readLine()
		if path == "" {
			break
		}

		file, err := readAttachment(path)
		if err != nil {
			log.Println(err)
			continue
		}

		msg.AddFile(file)
	}
	fmt.Println(msg)
	postMessage(msg)
}

func readAttachment(path string) (*fbb.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	name := filepath.Base(path)

	var resizeImage bool
	if isImageMediaType(name, "") {
		fmt.Print("This seems to be an image. Auto resize? [Y/n]: ")
		ans := readLine()
		resizeImage = ans == "" || strings.EqualFold("y", ans)
	}

	var data []byte

	if resizeImage {
		data, err = convertImage(f)
		ext := filepath.Ext(name)
		name = name[:len(name)-len(ext)] + ".jpg"
	} else {
		data, err = ioutil.ReadAll(f)
	}

	return fbb.NewFile(name, data), err
}

var stdin *bufio.Reader

func readLine() string {
	if stdin == nil {
		stdin = bufio.NewReader(os.Stdin)
	}

	str, _ := stdin.ReadString('\n')
	return strings.TrimSpace(str)
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

func getFormsVersion(templatePath string) string {
	// walking up the path to find a version file.
	// Winlink's Standard_Forms.zip includes it in its root.
	dir := templatePath
	if filepath.Ext(templatePath) == ".txt" {
		dir = filepath.Dir(templatePath)
	}

	var verFile *os.File
	// loop to walk up the subfolders until we find the top, or Winlink's Standard_Forms_Version.dat file
	for {
		f, err := os.Open(filepath.Join(dir, "Standard_Forms_Version.dat"))
		if err != nil {
			dir = filepath.Dir(dir) // have not found the version file or couldn't open it, going up by one
			if dir == "." || dir == ".." || strings.HasSuffix(dir, string(os.PathSeparator)) {
				return "unknown" // reached top-level and couldn't find version .dat file
			}
			continue
		}
		defer f.Close()
		// found and opened the version file
		verFile = f
		break
	}

	if verFile != nil {
		return readFileFirstLine(verFile)
	}
	return "unknown"
}

func readFileFirstLine(f *os.File) string {
	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		return scanner.Text()
	}
	return ""
}

func composeFormReport(args []string) {
	var tmplPathArg string

	set := pflag.NewFlagSet("form", pflag.ExitOnError)
	set.StringVar(&tmplPathArg, "template", "ICS USA Forms/ICS213", "")
	set.Parse(args)

	formFolder, err := buildFormFolder()
	if err != nil {
		log.Printf("can't build form folder tree %s", err)
		return
	}

	tmplPath := filepath.Clean(tmplPathArg)
	form, err := findFormFromURI(tmplPath, formFolder)
	if err != nil {
		log.Printf("can't find form to match form %s", tmplPath)
		return
	}

	msg := composeMessageHeader(nil)
	var varMap map[string]string
	varMap = make(map[string]string)
	varMap["subjectline"] = msg.Subject()
	varMap["templateversion"] = getFormsVersion(config.FormsPath)
	varMap["msgsender"] = fOptions.MyCall
	fmt.Println("forms version: " + varMap["templateversion"])

	formMsg, err := FormMessageBuilder {
		Template: form,
		FormValues: varMap,
		Interactive: true,
		IsReply: false,
	}.Build()

	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not open form file '%s'.\nRun 'pat configure' and verify that 'forms_path' is set up and the files exist.\n", tmplPath)
		os.Exit(1)
	}
	msg.SetSubject(formMsg.Subject)

	fmt.Println("================================================================")
	fmt.Print("To: ")
	fmt.Println(msg.To())
	fmt.Print("Cc: ")
	fmt.Println(msg.Cc())
	fmt.Print("From: ")
	fmt.Println(msg.From())
	fmt.Println("Subject: " + msg.Subject())
	fmt.Println(formMsg.Body)
	fmt.Println("================================================================")
	fmt.Println("Press ENTER to post this message in the outbox, Ctrl-C to abort.")
	fmt.Println("================================================================")
	readLine()

	msg.SetBody(formMsg.Body)

	attachmentName := GetXmlAttachmentNameForForm(form, false)
	attachmentFile := fbb.NewFile(attachmentName, []byte(formMsg.AttachmentXml))
	msg.AddFile(attachmentFile)

	postMessage(msg)
}

func GetXmlAttachmentNameForForm(f Form, isReply bool) string {
	attachmentName := filepath.Base(f.ViewerURI)
	if isReply {
		attachmentName = filepath.Base(f.ReplyViewerURI)
	}
	attachmentName = strings.TrimSuffix(attachmentName, filepath.Ext(attachmentName))
	attachmentName = "RMS_Express_Form_" + attachmentName + ".xml"
	if len(attachmentName) > 255 {
		attachmentName = strings.TrimPrefix(attachmentName, "RMS_Express_Form_")
	}
	return attachmentName
}

type FormMessageBuilder struct {
	Interactive bool
	IsReply     bool
	Template Form
	FormValues map[string]string
}

//returns message subject, body, and XML attachment content for the given template and variable map
func (b FormMessageBuilder) Build () (MessageForm, error) {

	tmplPath := filepath.Join(config.FormsPath, b.Template.TxtFileURI)
	if filepath.Ext(tmplPath) == "" {
		tmplPath += ".txt"
	}
	if b.IsReply && b.Template.ReplyTxtFileURI != "" {
		tmplPath = filepath.Join(config.FormsPath, b.Template.ReplyTxtFileURI)
	}

	var retVal MessageForm

	infile, err := os.Open(tmplPath)
	if err != nil {
		return retVal, err
	}

	placeholderRegEx := regexp.MustCompile(`<[vV][aA][rR]\s+(\w+)\s*>`)
	scanner := bufio.NewScanner(infile)

	for scanner.Scan() {
		lineTmpl := scanner.Text()
		lineTmpl = fillPlaceholders(lineTmpl, placeholderRegEx, b.FormValues)
		lineTmpl = strings.Replace(lineTmpl, "<MsgSender>", fOptions.MyCall, -1)
		lineTmpl = strings.Replace(lineTmpl, "<ProgramVersion>", "Pat "+versionStringShort(), -1)
		if strings.HasPrefix(lineTmpl, "Form:") ||
			strings.HasPrefix(lineTmpl, "ReplyTemplate:") ||
			strings.HasPrefix(lineTmpl, "To:") ||
			strings.HasPrefix(lineTmpl, "Msg:") {
			continue
		}
		if b.Interactive {
			matches := placeholderRegEx.FindAllStringSubmatch(lineTmpl, -1)
			fmt.Println(string(lineTmpl))
			for i := range matches {
				varName := matches[i][1]
				varNameLower := strings.ToLower(varName)
				if b.FormValues[varNameLower] != "" {
					continue
				}
				fmt.Print(varName + ": ")
				b.FormValues[varNameLower] = "blank"
				val := readLine()
				if val != "" {
					b.FormValues[varNameLower] = val
				}
			}
		}
		lineTmpl = fillPlaceholders(lineTmpl, placeholderRegEx, b.FormValues)
		if strings.HasPrefix(lineTmpl, "Subject:") {
			retVal.Subject = strings.TrimPrefix(lineTmpl, "Subject:")
		} else {
			retVal.Body += lineTmpl + "\n"
		}
	}
	infile.Close()

	if b.IsReply {
		b.FormValues["msgisreply"] = "True"
	} else {
		b.FormValues["msgisreply"] = "False"
	}
	b.FormValues["msgsender"] = fOptions.MyCall

	// some defaults that we can't set yet. Winlink doesn't seem to care about these
	b.FormValues["msgto"] = ""
	b.FormValues["msgcc"] = ""
	b.FormValues["msgsubject"] = ""
	b.FormValues["msgbody"] = ""
	b.FormValues["msgp2p"] = ""
	b.FormValues["msgisforward"] = "False"
	b.FormValues["msgisacknowledgement"] = "False"
	b.FormValues["msgseqnum"] = "0"

	formVarsAsXml := ""
	for varKey, varVal := range b.FormValues {
		formVarsAsXml += fmt.Sprintf("    <%s>%s</%s>\n", XmlEscape(varKey), XmlEscape(varVal), XmlEscape(varKey))
	}

	viewer := ""
	if b.Template.ViewerURI != "" {
		viewer = filepath.Base(b.Template.ViewerURI)
	}
	if b.IsReply && b.Template.ReplyViewerURI != "" {
		viewer = filepath.Base(b.Template.ReplyViewerURI)
	}

	replier := ""
	if !b.IsReply && b.Template.ReplyTxtFileURI != "" {
		replier = filepath.Base(b.Template.ReplyTxtFileURI)
	}

	retVal.AttachmentXml = fmt.Sprintf(`%s<RMS_Express_Form>
  <form_parameters>
    <xml_file_version>%s</xml_file_version>
    <rms_express_version>%s</rms_express_version>
    <submission_datetime>%s</submission_datetime>
    <senders_callsign>%s</senders_callsign>
    <grid_square>%s</grid_square>
    <display_form>%s</display_form>
    <reply_template>%s</reply_template>
  </form_parameters>
  <variables>
%s
  </variables>
</RMS_Express_Form>
`,
		xml.Header,
		"1.0",
		versionStringShort(),
		time.Now().UTC().Format("20060102150405"),
		fOptions.MyCall,
		config.Locator,
		viewer,
		replier,
		formVarsAsXml)

	retVal.Subject = strings.TrimSpace(retVal.Subject)
	retVal.Body = strings.TrimSpace(retVal.Body)

	return retVal, nil
}

func XmlEscape(s string) string {
	sEscaped := bytes.NewBuffer(make([]byte, 0))
	sEscapedStr := ""

	if err := xml.EscapeText(sEscaped, []byte(s)); err != nil {
		log.Printf("Error trying to escape XML string %s", err)
	} else {
		sEscapedStr = sEscaped.String()
	}
	return sEscapedStr
}

func fillPlaceholders(s string, re *regexp.Regexp, values map[string]string) string {
	if _, ok := values["txtstr"]; !ok {
		values["txtstr"] = ""
	}
	result := s
	matches := re.FindAllStringSubmatch(s, -1)
	for _, match := range matches {
		value, ok := values[strings.ToLower(match[1])]
		if ok {
			result = strings.Replace(result, match[0], value, -1)
		}
	}
	return result
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
	return fmt.Sprintf("v%s (%s) %s/%s - %s", Version, GitRev, runtime.GOOS, runtime.GOARCH, runtime.Version())
}

func versionStringShort() string {
	return fmt.Sprintf("v%s (%s)", Version, GitRev)
}
