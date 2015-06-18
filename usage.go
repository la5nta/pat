package main

var UsageConnect = `mode[:address]

Modes:
  winmor, ax25, telnet, serial-tnc.

Address syntax:
  winmor:       callsign[@frequency]
  telnet:       callsign[:password]@ip[:port] (or blank for CMS session)
  ax25:         callsign [via callsign]
  serial-tnc:   callsign [via callsign]
`
var ExampleConnect = `
  connect telnet                Connect to one of the Winlink Common Message Servers via internet.
  connect ax25:LA1B-10          Connect to the RMS Gateway LA1B-10 using Linux AX.25.
  connect ax25:LA5NTA via LA1B  Peer-to-peer connection with LA5NTA via LA1B digipeater.
  connect winmor:LA3F           Connect to the RMS HF Gateway LA3F using Winmor.
  connect winmor:LA3F@5350      Same as above, but set dial frequency of the radio using rigcontrol.
  connect serial-tnc:LA1B-10    Connect to the RMS Gateway LA1B-10 over a AX.25 serial TNC
`
