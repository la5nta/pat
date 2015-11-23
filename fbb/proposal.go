// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package fbb

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/la5nta/wl2k-go/lzhuf"
)

type PropCode byte

const (
	BasicProposal PropCode = 'B' // Basic ASCII proposal (or compressed binary in v0/1)
	AsciiProposal          = 'A' // Compressed v0/1 ASCII proposal
	Wl2kProposal           = 'C' // Compressed v2 proposal (winlink extension)
	GzipProposal           = 'D' // Gzip compressed v2 proposal
)

type ProposalAnswer byte

const (
	Accept ProposalAnswer = '+'
	Reject                = '-'
	Defer                 = '='

	// Offset not supported yet
)

// Proposal is the type representing a inbound or outbound proposal.
type Proposal struct {
	code           PropCode
	msgType        string
	mid            string
	answer         ProposalAnswer
	title          string
	offset         int
	sent           bool
	size           int
	compressedData []byte
	compressedSize int
}

// Constructor for a new Proposal given a Winlink Message.
//
// Reads the Winlink Message given and constructs a new proposal
// based on what's read and prepares for outbound delivery, returning
// a Proposal with the given data.
//
func NewProposal(MID, title string, code PropCode, data []byte) *Proposal {
	prop := &Proposal{
		mid:     MID,
		code:    code,
		msgType: "EM",
		title:   title,
		size:    len(data),
	}

	if prop.title == `` {
		prop.title = `No title`
	}

	if prop.code == GzipProposal {
		// Gzip compressed
		var buf bytes.Buffer
		z, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err != nil {
			panic(err)
		}

		z.Write(data)
		z.Close()
		prop.compressedData = buf.Bytes()
	} else {
		// LZHUF compressed
		prop.compressedData = lzhuf.Encode(data)
	}
	prop.compressedSize = len(prop.compressedData)

	return prop
}

// Method for checking if the Proposal is completely
// downloaded/loaded and ready to be read/sent.
//
// Typically used to check if the whole message was
// successfully downloaded from the CMS.
//
func (p *Proposal) DataIsComplete() bool {
	return len(p.compressedData) == p.compressedSize
}

// Returns the uniqe Message ID
func (p *Proposal) MID() string {
	return p.mid
}

// Returns the title of this proposal
func (p *Proposal) Title() string {
	return p.title
}

func (p *Proposal) Message() (*Message, error) {
	buf := bytes.NewBuffer(p.Data())
	m := new(Message)
	err := m.ReadFrom(buf)
	return m, err
}

// Data returns the decompressed raw message
func (p *Proposal) Data() []byte {
	if p.code == GzipProposal {
		return p.gzipData()
	}
	return lzhuf.Decode(p.compressedData) //TODO: Should return error when decompress fails
}

func (p *Proposal) gzipData() []byte {
	rd, err := gzip.NewReader(bytes.NewBuffer(p.compressedData))
	if err != nil {
		panic(err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, rd)
	return buf.Bytes()
}

func parseProposal(line string, prop *Proposal) (err error) {
	if len(line) < 1 {
		return
	} else if line[0] != 'F' {
		return
	}

	prop.code = PropCode(line[1])

	switch prop.code {
	case BasicProposal, AsciiProposal: // TODO: implement
	case Wl2kProposal, GzipProposal:
		err = parseB2Proposal(line, prop)
	default:
		err = fmt.Errorf("Unsupported proposal code '%c'", prop.code)
	}
	return
}

func parseB2Proposal(line string, prop *Proposal) (err error) {
	if len(line) < 4 {
		return errors.New("Unexpected end of proposal line")
	}

	if !(line[1] == Wl2kProposal || line[1] == GzipProposal) {
		return errors.New("Not a type C or D proposal")
	}

	// FC EM TJKYEIMMHSRB 527 123 0
	parts := strings.Split(line[3:], " ")
	if len(parts) < 5 {
		return errors.New(`Malformed proposal: ` + line[2:])
	}

	for i, part := range parts {
		switch i {
		case 0:
			if len(part) < 1 || len(part) > 2 {
				return errors.New(`Malformed proposal 0`)
			} else if part != "EM" && part != "CM" {
				return fmt.Errorf(`Expected message type CM or EM, but found %s`, part)
			}
			prop.msgType = part
		case 1:
			prop.mid = part
		case 2:
			prop.size, _ = strconv.Atoi(part)
		case 3:
			prop.compressedSize, _ = strconv.Atoi(part)
		case 4:
		default:
			return errors.New(fmt.Sprintf(`Too many parts in proposal: %+v`, parts))
		}
	}
	return
}
