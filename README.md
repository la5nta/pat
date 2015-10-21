# wl2k-go

wl2k-go is a collection of Go packages implementing various parts needed to build a Winlink 2000 client. It includes a Winlink client running on Linux, OS X and other unix-like platforms.

The project's goal is to encourage and facilitate development of cross-platform Winlink 2000 clients.

_This project is under heavy development and breaking API changes are to be expected._

## cmd/wl2k: The client application

cmd/wl2k is a (experimental) cross-platform Winlink client with basic messaging capabilities.

It is the primary sandbox/prototype application for the various wl2k-go sub-packages, and provides both a command line interface and a responsive (mobile-friendly) web interface.

It is mainly developed for Linux, but has also been tested on OS X and Android.

See [Building from source](https://github.com/LA5NTA/wl2k-go/wiki/Building-from-source) for build instructions.

#### Features
* Message composer/reader (basic mailbox functionality).
* Auto-shrink image attachments (EXPERIMENTAL).
* Post position reports (uses browser location/GPS in http mode).
* Rig control (using hamlib) for winmor PTT and QSY.
* CRON-like syntax for execution of scheduled commands (e.g. QSY or connect).
* Built in http-server with web interface (mobile friendly).
* Git style command line interface.
* Listen for P2P connections using multiple modes concurrently.
* AX.25, telnet, WINMOR and ARDOP support.
* Experimental gzip message compression (See "Gzip experiment" below).

##### Example
```
martinhpedersen@duo:~/wl2k-go$ wl2k interactive
> listen winmor,telnet-p2p,ax25
2015/02/03 10:33:10 Listening for incoming traffic (winmor,telnet-p2p,ax25)...
> connect winmor:LA3F
2015/02/03 10:34:28 Connecting to winmor:LA3F...
2015/02/03 10:34:33 Connected to WINMOR:LA3F
RMS Trimode 1.3.3.0 Follo.SE Oslo. Pactor & Winmor Hybrid Gateway
LA5NTA has 117 minutes remaining with LA3F
[WL2K-2.8.4.8-B2FWIHJM$]
Wien CMS via LA3F >
>FF
FC EM FOYNU8AKXX59 260 221 0
F> 68
1 proposal(s) received
Accepting FOYNU8AKXX59
Receiving [//WL2K test til linux] [offset 0]
>FF
FQ
Waiting for remote node to close the connection...
> _
```

## wl2k: Winlink 2000/B2F protocol implementation

An implementation of the B2 Forwarding Protocol and Winlink 2000 Message Structure (the WL2K-protocol).

```go
mycall := "LA5NTA"
mbox := mailbox.NewDirHandler("/tmp/mailbox", false)
session := wl2k.NewSession(
	mycall,
	telnet.TargetCall,
	"JP20qh",
	mbox, // Use /tmp/mailbox as the mailbox for this session
)

// Exchange messages over any connection implementing the net.Conn interface
conn, _ := telnet.Dial(mycall)
session.Exchange(conn)

// Print subjects of messages in the inbox
msgs, _ := mbox.Inbox()
for _, msg := range msgs {
	fmt.Printf("Have message: %s\n", msg.Subject())
}
```

For detailed package documentation, see <http://godoc.org/github.com/la5nta/wl2k-go>.

A big thanks to paclink-unix by Nicholas S. Castellano N2QZ (and others). Without their effort and choice to share their knowledge through open source code, this implementation would probably never exist.

Paclink-unix was used as reference implementation for the B2F protocol since the start of this project.

### Gzip experiment

Gzip message compression has been added as an experimental B2F extension, as an alternative to LZHUF. The feature can be enabled by setting the environment variable `GZIP_EXPERIMENT=1` at runtime.

The protocol extension is negotiated by an additional character (G) in the handshake SID as well as a new proposal code (D), thus making it backwards compatible with software not supporting gzip compression.

The G sid flag tells the other party that gzip is supported through a D-proposal. The D-proposal has the same format as C-proposals, but is used to flag the data as gzip compressed.

The gzip feature works transparently, which means that it will not break protocol if it's unsupported by the other winlink node.

## lzhuf: The compression

This project does not currently implement the lzhuf compression algorithm required. It does however provide a Go wrapper (and a minor patch + install script) for using JNOS's code (http://www.langelaar.net/projects/jnos2). To fetch the source code and apply the provided patch:

```bash
cd lzhuf;make;cd ..
```
That's it!

Thanks to the JNOS contributors, Jack Snodgrass and others :-)

For detailed package documentation, see <http://godoc.org/github.com/la5nta/wl2k-go/lzhuf>.

## transport

Package transport provides access to various connected modes commonly used for winlink.

The modes is made available through common interfaces and idioms from the net package, mainly net.Conn and net.Listener.

For detailed package documentation, see <http://godoc.org/github.com/la5nta/wl2k-go/transport>.

#### telnet
* A simple TCP dialer/listener for the "telnet"-method.
* Supports both P2P and CMS dialing.

#### ax25
* Wrapper for the Linux AX.25 library (build with tag "libax25").
* Kenwood TH-D7x/TM-D7x0 (or similar) TNCs over serial.

#### winmor
A WINMOR TNC driver that provides dialing and listen capabilities for a local or remote TNC.

The WINMOR TNC can be run under Wine:
* Tested OK with WINMOR TNC 1.5.7.0 running on wine 1.6.2-17 (debian jessie) with .NET 2.0, 3.0 and 3.5 installed.
* Tested OK with WINMOR TNC 1.4.7.0 running on wine 1.4.1-4 (debian wheezy) with .NET 2.0, 3.0, 3.5 and 4.0 installed.

When running WINMOR TNC under wine through pulseaudio, set PULSE_LATENCY_MSEC=60.

#### ardop
A driver for the ARDOP_Win (alpha) TNC that provides dialing and listen capabilities over ARDOP (Amateur Radio Digital Open Protocol).

## mailbox: Directory based MBoxHandler implementation

For detailed package documentation, see <http://godoc.org/github.com/la5nta/wl2k-go/mailbox>.

```go
mbox := mailbox.NewDirHandler("/tmp/mailbox", false)

session := wl2k.NewSession(
    "N0CALL",
    telnet.TargetCall,
    "JP20qh",
    mbox,
)
```

## rigcontrol/hamlib

Go bindings for a _subset_ of hamlib. It provides both native cgo bindings and a rigctld client.

Build with `-tags libhamlib` to link against libhamlib (the native library).

See <http://godoc.org/github.com/LA5NTA/wl2k-go/rigcontrol/hamlib> for more details.

## Copyright/License

Copyright (c) 2014-2015 Martin Hebnes Pedersen LA5NTA

(See LICENSE)

## Thanks to

The JNOS developers for the properly maintained lzhuf implementation, as well as the original author Haruyasu Yoshizaki.

The paclink-unix team (Nicholas S. Castellano N2QZ and others) - reference implementation

Amateur Radio Safety Foundation, Inc. - The Winlink 2000 project

F6FBB Jean-Paul ROUBELAT - the FBB forwarding protocol

### Contributors (alphabetical)

* LA3QMA - Kai Günter Brandt
* LA5NTA - Martin Hebnes Pedersen

_wl2k-go is not affiliated with The Winlink Development Team nor the Winlink 2000 project [http://winlink.org]._
