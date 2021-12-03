<a href="http://getpat.io"><img src="https://raw.githubusercontent.com/la5nta/pat-website/gh-pages/img/logo.png" width="128" ></a>

[![Build Status](https://travis-ci.com/la5nta/pat.svg?branch=master)](https://travis-ci.com/la5nta/pat)
[![Windows Build Status](https://ci.appveyor.com/api/projects/status/tstq4suxfdmudl5l/branch/master?svg=true)](https://ci.appveyor.com/project/martinhpedersen/pat)
[![Go Report Card](https://goreportcard.com/badge/github.com/la5nta/pat)](https://goreportcard.com/report/github.com/la5nta/pat)
[![Liberapay Patreons](http://img.shields.io/liberapay/patrons/la5nta.svg?logo=liberapay)](https://liberapay.com/la5nta)

## Overview

Pat is a cross platform Winlink client with basic messaging capabilities.

It is the primary sandbox/prototype application for the [wl2k-go](https://github.com/la5nta/wl2k-go) project, and provides both a command line interface and a responsive (mobile-friendly) web interface.

It is mainly developed for Linux, but is also known to run on OS X, Windows and Android.

#### Features
* Message composer/reader (basic mailbox functionality).
* Auto-shrink image attachments.
* Post position reports with location from local GPS, browser location or manual entry.
* Rig control (using hamlib) for winmor PTT and QSY.
* CRON-like syntax for execution of scheduled commands (e.g. QSY or connect).
* Built in http-server with web interface (mobile friendly).
* Git style command line interface.
* Listen for P2P connections using multiple modes concurrently.
* AX.25, telnet, WINMOR and ARDOP support.
* Experimental gzip message compression (See "Gzip experiment" below).

##### Example
```
martinhpedersen@duo:~$ pat interactive
> listen winmor,telnet-p2p,ax25
2015/02/03 10:33:10 Listening for incoming traffic (winmor,telnet-p2p,ax25)...
> connect winmor:///LA3F
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

### Gzip experiment

Gzip message compression has been added as an experimental B2F extension. The extension is implemented as a backwards compatible alternative to the ancient LZHUF compression.

This experiment is enabled by default and sessions between two Pat nodes (or other software supporting this B2F extension) will use gzip compression when transferring messages.

For more information, see <https://github.com/la5nta/wl2k-go#gzip-experiment>.

## Copyright/License

Copyright (c) 2020 Martin Hebnes Pedersen LA5NTA

### Contributors (alphabetical)

* DL1THM - Torsten Harenberg
* HB9GPA - Matthias Renner
* K0SWE - Chris Keller
* KD8DRX - Will Davidson
* KE8HMG - Andrew Huebner
* KI7RMJ - Rainer Grosskopf
* LA3QMA - Kai Günter Brandt
* LA4TTA - Erlend Grimseid
* LA5NTA - Martin Hebnes Pedersen
* W6IPA  - JC Martin
* WY2K - Benjamin Seidenberg
* VE7GNU - Doug Collinge

## Thanks to

The JNOS developers for the properly maintained lzhuf implementation, as well as the original author Haruyasu Yoshizaki.

The paclink-unix team (Nicholas S. Castellano N2QZ and others) - reference implementation

Amateur Radio Safety Foundation, Inc. - The Winlink 2000 project

F6FBB Jean-Paul ROUBELAT - the FBB forwarding protocol

_Pat/wl2k-go is not affiliated with The Winlink Development Team nor the Winlink 2000 project [http://winlink.org]._
