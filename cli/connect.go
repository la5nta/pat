package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/la5nta/pat/app"
)

func ConnectHandle(_ context.Context, app *app.App, args []string) {
	if args[0] == "" {
		fmt.Println("Missing argument, try 'connect help'.")
	}

	app.PromptHub().AddPrompter(TerminalPrompter{})

	if success := app.Connect(args[0]); !success {
		os.Exit(1)
	}
}

const (
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
  ?prehook=     Sets an executable middleware to run before the connection is handed over to the B2F protocol.
                 The executable must be given as full path, or a file located in $PATH or {CONFIG_DIR}/prehooks/.
		 Received packets are forwarded to STDIN. Data written to STDOUT forwarded to the remote node.
		 Additional arguments can be passed with one or more &prehook-arg=.
		 Environment variables describing the dialed connection are provided.
                 Useful for packet node traversal. Supported across all transports.
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
