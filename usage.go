// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

var (
	UsageConnect = `'alias' or 'transport://[host][/digi]/targetcall[?params...]'

transport:
  telnet:          TCP/IP
  ardop:           ARDOP TNC
  pactor:          SCS PTC modems
  varahf:          VARA HF TNC
  varafm:          VARA FM TNC
  ax25:            AX.25 (Default - uses engine specified in config)
  ax25+agwpe:      AX.25 (AGWPE/Direwolf)
  ax25+linux:      AX.25 (Linux kernel)
  ax25+serial-tnc: AX.25 (Serial TNC)

host:
  Used to address the host interface (TNC/modem), _not_ to be confused with the connection PATH.
    Format: [user[:pass]@]host[:port]

  telnet:       [user:pass]@host:port
  ax25+linux:   (optional) host=axport
  pactor:       (optional) serial device (e.g. COM1 or /dev/ttyUSB0)

path:
  The last element of the path is the target station's callsign. If the path has
   multiple hops (e.g. AX.25), they are separated by '/'.

params:
  ?freq=        Sets QSY frequency (ardop and ax25 only)
  ?host=        Overrides the host part of the path. Useful for serial-tnc to specify e.g. /dev/ttyS0.
`
	ExampleConnect = `
  connect telnet                       (alias) Connect to one of the Winlink Common Message Servers via tcp.
  connect ax25:///LA1B-10              Connect to the RMS Gateway LA1B-10 using AX.25 engine as per configuration.
  connect ax25+linux://tmd710/LA1B-10  Connect to LA1B-10 using Linux kernel's AX.25 stack on axport 'tmd710'.
  connect ax25:///LA1B/LA5NTA          Peer-to-peer connection with LA5NTA via LA1B digipeater.
  connect ardop:///LA3F                Connect to the RMS HF Gateway LA3F using ARDOP on the default tcp address and port.
  connect ardop:///LA3F?freq=5350      Same as above, but set dial frequency of the radio using rigcontrol.  
  connect pactor:///LA3F               Connect to RMS HF Gateway LA3F using PACTOR.
  connect varahf:///LA1B               Connect to RMS HF Gateway LA1B using VARA HF TNC.
  connect varafm:///LA5NTA             Connect to LA5NTA using VARA FM TNC.
`
)

var ExamplePosition = `
  position -c "QRV 145.500MHz"       Send position and comment with coordinates retrieved from GPSd.
  position --latlon 59.123,005.123   Send position 59.123N 005.123E.
  position --latlon 40.704,-73.945   Send position 40.704N 073.945W.
  position --latlon -10.123,-60.123  Send position 10.123S 060.123W.
`
