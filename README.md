# wl2k-go

wl2k-go is a collection of go packages that implement various parts needed to build a Winlink 2000 client.

The project's goal is to encourage and faciliate development of cross-platform Winlink 2000 clients.

wl2k-go is not affiliated with The Winlink Development Team nor the Winlink 2000 project [http://winlink.org].

_This project is under heavy development and breaking API changes are to be expected._

## wl2k - Winlink 2000/B2F protocol implementation

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

A big thanks to paclink-unix by Nicholas S. Castellano N2QZ (and others). Without their effort and choice to share their knowledge through open source code, this implementation would probably never exist.

Paclink-unix was used as reference implementation for the B2F protocol since the start of this project.

## cmd/wl2k - A command line Winlink client

cmd/wl2k implements a fully working command-line and responsive (mobile-friendly) webapp Winlink-client with support for various connection methods. It supports some minimalistic mailbox functionality (read/compose/extract emails).

```
martinhpedersen@duo:~/wl2k-go$ wl2k -interactive
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

Connection methods:

* telnet
* WINMOR TNC
* AX.25 (Linux)
* Kenwood TH-D7x/TM-D7x0

Listen (P2P) methods:

* telnet
* Winmor TNC
* AX.25 (Linux)

Build with "-tags libax25" to get libax25 support in Linux.

See lzhuf section for how to prepare that package.

Other dependencies: libhamlib-dev, libreadline-dev and optionally imagemagick (for auto resize of image attachments)

To install: `go install -tags libax25 github.com/martinhpedersen/wl2k-go/wl2k`

For simple usage try `wl2k --help`

A configuration file is automatically created on the first run at `$HOME/.wl2k/config.json`.

## lzhuf - the compression

This project does not currently implement the lzhuf compression algorithm required. It does however provide a go wrapper (and a minor patch + install script) for using JNOS's code (http://www.langelaar.net/projects/jnos2). To fetch the source code and apply the provided patch:

```bash
cd lzhuf;make;cd ..
```
That's it!

Thanks to the JNOS contributors, Jack Snodgrass and others :-)

## transport/telnet - net.Conn interface for connecting to the Winlink 2000 System over tcp

A simple TCP dialer/listener for the "telnet"-method.

## transport/ax25 - net.Conn interface for AX.25 connections

Various implementations of the net.Conn interface for AX.25 connections:

* Wrapper for the Linux AX.25 library (build with tag "libax25")
* Kenwood TH-D7x/TM-D7x0 (or similar) TNC's over serial

## transport/winmor - net.Conn connection to a remote node using a WINMOR TNC

Provides means of controlling and using a WINMOR TNC (local or over the network).

TIP: The WINMOR TNC can be run under Wine!

* Tested OK with WINMOR TNC 1.5.7.0 running on wine 1.6.2-17 (debian jessie) with .NET 2.0, 3.0 and 3.5 installed.
* Tested OK with WINMOR TNC 1.4.7.0 running on wine 1.4.1-4 (debian wheezy) with .NET 2.0, 3.0, 3.5 and 4.0 installed.

When running WINMOR TNC under wine through pulseaudio, set PULSE_LATENCY_MSEC=60.

## mailbox - Directory based MBoxHandler implementation

```go
mbox := mailbox.NewDirHandler("/tmp/mailbox", false)

session := wl2k.NewSession(
    "N0CALL",
    telnet.TargetCall,
    "JP20qh",
    mbox,
)
```

## Copyright/License

Copyright (c) 2014-2015 Martin Hebnes Pedersen LA5NTA

(See LICENSE)

## Thanks to

The JNOS developers for the properly maintained lzhuf implementation, as well as the original author Haruyasu Yoshizaki.

The paclink-unix team (Nicholas S. Castellano N2QZ and others) - reference implementation

Amateur Radio Safety Foundation, Inc. - The Winlink 2000 project

F6FBB Jean-Paul ROUBELAT - the FBB forwarding protocol
