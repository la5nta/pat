// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

var UsageConnect = `'alias' or 'transport://[host][/digi]/targetcall[?params...]'

transport:
  winmor:     WINMOR TNC
  ardop:      ARDOP TNC
  ax25:       AX.25 (Linux only)
  telnet:     TCP/IP
  serial-tnc: Serial AX.25 TNC

host:
  Used to address the host interface (TNC/modem), _not_ to be confused with the connection PATH.
    Format: [user[:pass]@]host[:port]

  telnet:       [user:pass]@host:port
  ax25:         (optional) host=axport

path:
  The last element of the path is the target station's callsign. If the path has
   multiple hops (e.g. AX.25), they are separated by '/'.

params:
  ?freq=        Sets QSY frequency (winmor, ardop and ax25 only)
  ?host=        Overrides the host part of the path. Useful for serial-tnc to specify e.g. /dev/ttyS0.
`
var ExampleConnect = `
  connect telnet                     (alias) Connect to one of the Winlink Common Message Servers via tcp.
  connect ax25:///LA1B-10            Connect to the RMS Gateway LA1B-10 using Linux AX.25 on the default axport.
  connect ax25://tmd710/LA1B-10      Connect to the RMS Gateway LA1B-10 using Linux AX.25 on axport 'tmd710'.
  connect ax25:///LA1B/LA5NTA        Peer-to-peer connection with LA5NTA via LA1B digipeater.
  connect winmor:///LA3F             Connect to the RMS HF Gateway LA3F using WINMOR TNC on default tcp address and port.
  connect winmor:///LA3F?freq=5350   Same as above, but set dial frequency of the radio using rigcontrol.
  connect ardop:///LA3F              Connect to the RMS HF Gateway LA3F using ARDOP on the default tcp address and port.
  connect ardop:///LA3F?freq=5350    Same as above, but set dial frequency of the radio using rigcontrol.  
  connect serial-tnc:///LA1B-10      Connect to the RMS Gateway LA1B-10 over a AX.25 serial TNC on the default serial port.
`
