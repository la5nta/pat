// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// A fully working Winlink 2000 command line client with support for various connection methods.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

var (
	fIgnoreBusy  bool
	fMyCall      string
	fListen      string
	fConnect     string
	fMailboxPath string
	fConfigPath  string
	fLogPath     string
	fExtract     string
	fCompose     bool
	fSendOnly    bool
	fReadMail    bool
	fInteractive bool
	fRigList     bool
	fPosReport   string
	fHttp        string
)

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

/*
wl2k -mycall LA5NTA -listen winmor,telnet,ax25
wl2k -mycall LA5NTA -connect winmor:LE1OF
wl2k -mycall LA5NTA -connect ax25:LE1OF
wl2k -mycall LA5NTA -connect serial-tnc:LE1OF
wl2k -mycall LA5NTA -connect telnet:[callsign(:password)]@[ip]:[port]
wl2k -mycall LA5NTA -connect telnet // For wl2k CMS
wl2k -interactive
wl2k -compose
wl2k -extract ~/foo/123ASDOIJD
wl2k -pos-report "60.1:005.3:I'm OK"
*/

func init() {
	listeners = make(map[string]net.Listener)

	defaultMBox, _ := mailbox.DefaultMailboxPath()

	if appDir, err := mailbox.DefaultAppDir(); err != nil {
		log.Fatal(err)
	} else {
		fConfigPath = path.Join(appDir, "config.json")
		fLogPath = path.Join(appDir, "wl2k.log")
	}

	flag.StringVar(&fMyCall, `mycall`, ``, `Your callsign (winlink user)`)
	flag.StringVar(&fListen, "listen", "", "Listen mode: Comma-separated list of methods to listen on. Ie. winmor,telnet,ax25")
	flag.StringVar(&fConnect, "connect", "", "Connect mode: Method and target to use in the form [method:target call]. Ie. winmor:N0CALL")
	flag.BoolVar(&fInteractive, "interactive", false, "Interactive mode")
	flag.StringVar(&fMailboxPath, "mbox", defaultMBox, "Path to mailbox directory")
	flag.StringVar(&fConfigPath, "config", fConfigPath, "Path to config file")
	flag.StringVar(&fLogPath, "log", fLogPath, "Path to log file. The file is truncated on each startup.")
	flag.BoolVar(&fSendOnly, `send-only`, false, `Download inbound messages later, send only.`)
	flag.StringVar(&fExtract, `extract`, ``, `Extract email given and exit`)
	flag.BoolVar(&fCompose, `compose`, false, `Compose email and exit`)
	flag.BoolVar(&fReadMail, "read", false, "Read emails (interactive mailbox browser)")
	flag.BoolVar(&fIgnoreBusy, "ignore-busy", false, "Don't wait for clear channel before connecting to a node")
	flag.StringVar(&fPosReport, "pos-report", "", "Prepare a position report message from this position (format [lat]:[lon]:[comment] ie. 60.1:005.3:I'm OK)")
	flag.BoolVar(&fRigList, "rig-list", false, "List hamlib rig models (use in config for winmor PTT control)")
	flag.StringVar(&fHttp, "http", "", "Address to listen for http connections (ie. :8080)")

	// Logger
	f, err := os.Create(fLogPath)
	if err != nil {
		log.Fatal(err)
	}
	logWriter = io.MultiWriter(f, os.Stdout)
	log.SetOutput(logWriter)
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

func main() {
	defer cleanup()

	flag.Parse()
	var err error

	config, err = LoadConfig(fConfigPath, cfg.DefaultConfig)
	if err != nil {
		log.Fatalf("Unable to load/write config: %s", err)
	}

	if fMyCall == "" && config.MyCall == "" {
		fmt.Println("Missing mycall")
		return
	} else if fMyCall == "" {
		fMyCall = config.MyCall
	}

	if fListen == "" && len(config.Listen) > 0 {
		fListen = strings.Join(config.Listen, ",")
	}

	rigs = loadHamlibRigs()

	loadMBox()
	exchangeChan = exchangeLoop()

	switch {
	case fRigList:
		for m, str := range hamlib.Rigs() {
			fmt.Printf("%d\t%s\n", m, str)
		}
		return

	case len(fExtract) > 0:
		extractEmail(fExtract)
		return

	case len(fMyCall) == 0:
		log.Fatal("Missing -mycall")

	case fReadMail:
		readMail()
		return

	case fCompose:
		composeEmail(nil)
		return

	case len(fPosReport) > 0:
		posReport()
		return

	case fConnect != "":
		Connect(fConnect)
		return

	case fListen != "":
		Listen(fListen)
	}

	if fHttp != "" {
		go ListenAndServe(fHttp)
	}
	scheduleLoop()

	switch {
	case fInteractive:
		Interactive()
	case fListen != "" || fHttp != "":
		c := make(chan os.Signal)
		signal.Notify(c, os.Interrupt)
		<-c
		log.Println("Shutting down...")
	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Need -listen, -connect, -interactive, -compose, -pos-report or -extract. (See -help)")
}

func loadMBox() {
	mbox = mailbox.NewDirHandler(
		path.Join(fMailboxPath, fMyCall),
		fSendOnly,
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
	wmTNC, err = winmor.Open(config.Winmor.Addr, fMyCall, config.Locator)
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

func extractEmail(path string) {
	file, _ := os.Open(path)
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
	}
	return "editor"
}

func composeEmail(replyMsg *wl2k.Message) {
	msg := wl2k.NewMessage(fMyCall)
	in := bufio.NewReader(os.Stdin)

	fmt.Printf(`From [%s]: `, fMyCall)
	from := readLine(in)
	if from == "" {
		from = fMyCall
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
			if !addr.EqualString(fMyCall) {
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

	f, err := ioutil.TempFile("", "wl2k_new")
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

func posReport() {
	parts := strings.Split(fPosReport, ":")
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
	}.Message(fMyCall)

	postMessage(msg)
}

func postMessage(msg *wl2k.Message) {
	if err := mbox.AddOut(msg); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Message posted")
}
