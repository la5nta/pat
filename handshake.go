// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package wl2k

import (
	"bufio"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var ErrNoFB2 = errors.New("Remote does not support B2 Forwarding Protocol")

func (s *Session) handshake(rw io.ReadWriter) error {
	if s.master {
		if err := s.sendHandshake(rw, ""); err != nil {
			return err
		}
	}

	hs, err := s.readHandshake()
	if err != nil {
		return err
	}

	// Did we get SID codes?
	if hs.SID == "" {
		return errors.New("No sid in handshake")
	}

	s.remoteSID = hs.SID
	s.remoteFW = hs.FW

	var secureResp string
	if hs.SecureChallenge != "" {
		if s.secureLoginHandleFunc == nil {
			return errors.New("Got secure login challenge, please register a SecureLoginHandleFunc.")
		}

		password, err := s.secureLoginHandleFunc()
		if err != nil {
			return err
		}

		secureResp = secureLoginResponse(hs.SecureChallenge, password)
	}

	if !s.master {
		return s.sendHandshake(rw, secureResp)
	} else {
		return nil
	}
}

type handshakeData struct {
	SID             sid
	FW              []Address
	SecureChallenge string
}

func (s *Session) readHandshake() (handshakeData, error) {
	data := handshakeData{}

	for {
		bytes, err := s.rd.Peek(1)
		if err != nil {
			return data, err
		} else if bytes[0] == 'F' {
			return data, nil // Next line is a protocol command, handshake is done
		}

		// Ignore remote errors here, as the server sometimes sends lines like
		// '*** MTD Stats Total connects = 2580 Total messages = 3900', which
		// are not errors
		line, err := s.nextLineRemoteErr(false)
		if err != nil {
			return data, err
		}

		//REVIEW: We should probably be more strict on what to allow here,
		// to ensure we disconnect early if the remote is not talking the expected
		// protocol.
		switch {
		case strings.Contains(line, `[`): // Header with sid (ie. [WL2K-2.8.4.8-B2FWIHJM$])
			data.SID, err = parseSID(line)
			if err != nil {
				return data, err
			}

			// Do we support the remote's SID codes?
			if !data.SID.Has(sFBComp2) { // We require FBB compressed protocol v2 for now
				return data, ErrNoFB2
			}

		// ; lines wl2k specific commands?
		case strings.HasPrefix(line, ";FW"):
			data.FW, err = parseFW(line)
			if err != nil {
				return data, err
			}

		case strings.HasPrefix(line, ";PQ"):
			data.SecureChallenge = line[5:]
			//return data, errors.New("Got secure challenge by remote. Secure login not implemented.")
		}

		if strings.HasSuffix(line, ">") {
			return data, nil
		}
	}
}

// This salt was found in paclink-unix's source code.
var winlinkSecureSalt = []byte{
	77, 197, 101, 206, 190, 249,
	93, 200, 51, 243, 93, 237,
	71, 94, 239, 138, 68, 108,
	70, 185, 225, 137, 217, 16,
	51, 122, 193, 48, 194, 195,
	198, 175, 172, 169, 70, 84,
	61, 62, 104, 186, 114, 52,
	61, 168, 66, 129, 192, 208,
	187, 249, 232, 193, 41, 113,
	41, 45, 240, 16, 29, 228,
	208, 228, 61, 20}

// This algorithm for generating a secure login response token has been ported
// to Go from the paclink-unix implementation.
func secureLoginResponse(challenge, password string) string {
	payload := strings.ToUpper(challenge+password) + string(winlinkSecureSalt)

	sum := md5.Sum([]byte(payload))

	pr := int32(sum[3] & 0x3f)
	for i := 2; i >= 0; i-- {
		pr = (pr << 8) | int32(sum[i])
	}

	str := fmt.Sprintf("%08d", pr)

	return str[len(str)-8:]
}

func (s *Session) sendHandshake(writer io.Writer, secureResp string) error {
	w := bufio.NewWriter(writer)

	// Request messages on behalf of every localFW
	fmt.Fprintf(w, ";FW:")
	for _, addr := range s.localFW {
		fmt.Fprintf(w, " %s", addr.Addr)
	}
	fmt.Fprintf(w, "\r")

	writeSID(w, s.ua.Name, s.ua.Version)

	if secureResp != "" {
		writeSecureLoginResponse(w, secureResp)
	}

	fmt.Fprintf(w, "; %s DE %s (%s)", s.targetcall, s.mycall, s.locator)
	if s.master {
		fmt.Fprintf(w, ">\r")
	} else {
		fmt.Fprintf(w, "\r")
	}

	return w.Flush()
}

func parseFW(line string) ([]Address, error) {
	if !strings.HasPrefix(line, ";FW: ") {
		return nil, errors.New("Malformed forward line")
	}

	fws := strings.Split(line[5:], " ")
	addrs := make([]Address, 0, len(fws))

	for _, str := range strings.Split(line[5:], " ") {
		addrs = append(addrs, AddressFromString(str))
	}

	return addrs, nil
}

type sid string

const localSID = sFBComp2 + sFBBasic + sHL + sMID + sBID

// The SID codes
const (
	sAckForPM   = "A"  // Acknowledge for person messages
	sFBBasic    = "F"  // FBB basic ascii protocol supported
	sFBComp0    = "B"  // FBB compressed protocol v0 supported
	sFBComp1    = "B1" // FBB compressed protocol v1 supported
	sFBComp2    = "B2" // FBB compressed protocol v2 (aka B2F) supported
	sHL         = "H"  // Hierachical Location designators supported
	sMID        = "M"  // Message identifier supported
	sCompBatchF = "X"  // Compressed batch forwarding supported
	sI          = "I"  // "Identify"? Palink-unix sends ";target de mycall QTC n" when remote has this
	sBID        = "$"  // BID supported (must be last character in SID)
)

func writeSID(w io.Writer, appName, appVersion string) error {
	_, err := fmt.Fprintf(w, "[%s-%s-%s]\r", appName, appVersion, localSID)
	return err
}

func writeSecureLoginResponse(w io.Writer, response string) error {
	_, err := fmt.Fprintf(w, ";PR: %s\r", response)
	return err
}

func parseSID(str string) (sid, error) {
	code := regexp.MustCompile(`\[.*-(.*)\]`).FindStringSubmatch(str)
	if len(code) != 2 {
		return sid(""), errors.New(`Bad SID line: ` + str)
	}

	return sid(
		strings.ToUpper(code[len(code)-1]),
	), nil
}

func (s sid) Has(code string) bool {
	return strings.Contains(string(s), strings.ToUpper(code))
}
