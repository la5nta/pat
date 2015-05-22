// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package wl2k

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"strconv"
	"strings"
	"time"
)

const (
	HEADER_MID     = `MID`
	HEADER_TO      = `To`
	HEADER_DATE    = `Date`
	HEADER_TYPE    = `Type`
	HEADER_FROM    = `From`
	HEADER_CC      = `Cc`
	HEADER_SUBJECT = `Subject`
	HEADER_MBO     = `Mbo`
	HEADER_BODY    = `Body`
	HEADER_FILE    = `File`

	// These headers are stripped by the winlink system, but let's
	// include it anyway... just in case the winlink team one day
	// starts taking encoding seriously.
	HEADER_CONTENT_TYPE              = `Content-Type`
	HEADER_CONTENT_TRANSFER_ENCODING = `Content-Transfer-Encoding`

	// Internal headers
	HEADER_X_P2P_ONLY = `X-P2POnly`

	// The default body charset seems to be ISO-8859-1
	//
	// The Winlink Message Structure docs says that the body should
	// be ASCII-only, but RMS Express seems to encode the body as
	// ISO-8859-1. This is also the charset set (Content-Type header)
	// when a message reaches an SMTP server.
	DefaultCharset = "ISO-8859-1"

	// Mails going out over SMTP from the Winlink system is sent
	// with the header 'Content-Transfer-Encoding: 7bit', but
	// let's be reasonable...
	DefaultTransferEncoding = "8bit"

	// The date (in UTC) format as described in the Winlink
	// Message Structure docs (YYYY/MM/DD HH:MM).
	DateLayout = `2006/01/02 15:04`
)

// Representation of a receiver/sender address
type Address struct {
	Proto string
	Addr  string
}

// File represents an attachment
type File struct {
	data     []byte
	name     string
	dataSize int
}

// Message represent the Winlink 2000 Message Structure
// as defined in http://winlink.org/B2F
type Message struct {
	MID     string
	Date    time.Time
	From    Address
	To      []Address
	Cc      []Address
	Subject string
	Type    string
	Mbo     string
	Body    Body
	Files   []*File

	P2POnly bool
}

// NewMessage returns a new "private" message with
// MID, Date, Type, From and Mbo set.
func NewMessage(mycall string) *Message {
	return &Message{
		MID:  GenerateMid(mycall),
		Date: time.Now(),
		Type: `Private`,
		From: AddressFromString(mycall),
		Mbo:  mycall,
	}
}

// Returns true if the given Address is the _only_ receiver
// of this Message.
func (m *Message) IsOnlyReceiver(addr Address) bool {
	receivers := m.Receivers()
	if len(receivers) != 1 {
		return false
	}
	return receivers[0].String() == addr.String()
}

// Method for generating a proposal of the message
func (m *Message) Proposal() *Proposal {
	return NewProposal(
		m.MID,
		m.Subject,
		m.Bytes(),
	)
}

// Receivers returns a slice of all receivers of this message.
func (m *Message) Receivers() []Address {
	addrs := make([]Address, 0, len(m.To)+len(m.Cc))
	if len(m.To) > 0 {
		addrs = append(addrs, m.To...)
	}
	if len(m.Cc) > 0 {
		addrs = append(addrs, m.Cc...)
	}
	return addrs
}

// Add an attachment
func (m *Message) AddFile(f *File) {
	m.Files = append(m.Files, f)
}

// Bytes returns the message in the Winlink Message format.
func (m *Message) Bytes() []byte {
	var buf bytes.Buffer
	if _, err := m.WriteTo(&buf); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// Implements ReaderFrom for Message.
//
// Reads the given io.Reader and fills in values fetched from the stream.
func (m *Message) ReadFrom(r io.Reader) error {
	reader := bufio.NewReader(r)

	// MID must be at the top
	if mid, err := reader.ReadString('\n'); err != nil {
		return errors.New(`Unable to read from input`)
	} else if mid, err := readHeader(mid, HEADER_MID+`: `); err != nil {
		return err
	} else {
		m.MID = mid
	}

	charset := DefaultCharset

	// Parse address header
	var bodySize int
	done := false
	for !done {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		key, value := parseHeaderLine(line)
		switch lc(key) {
		case lc(HEADER_TO):
			m.To = append(m.To, AddressFromString(value))
		case lc(HEADER_CC):
			m.Cc = append(m.Cc, AddressFromString(value))
		case lc(HEADER_DATE):
			var err error
			if m.Date, err = time.Parse(DateLayout, value); err != nil {
				return err
			}
		case lc(HEADER_MBO):
			m.Mbo = value
		case lc(HEADER_TYPE):
			m.Type = value
		case lc(HEADER_FROM):
			m.From = AddressFromString(value)
		case lc(HEADER_SUBJECT):
			//REVIEW:
			//  This is documented as ASCII only, but
			//  RMS Express encodes it using latin1?
			m.Subject = value
		case lc(HEADER_CONTENT_TYPE):
			_, params, err := mime.ParseMediaType(value)
			if err != nil {
				continue
			} else if v, ok := params["charset"]; ok {
				charset = v
			}
		case lc(HEADER_BODY):
			bodySize, _ = strconv.Atoi(value)
		case lc(HEADER_FILE):
			slice := strings.SplitN(value, ` `, 2)
			if len(slice) < 2 {
				return errors.New(`Failed to parse file header. Got: ` + value)
			} else {
				size, _ := strconv.Atoi(slice[0])
				m.Files = append(m.Files, &File{
					name:     slice[1],
					dataSize: size,
				})
			}
		case lc(HEADER_X_P2P_ONLY):
			if b, err := strconv.ParseBool(value); err != nil {
				return fmt.Errorf("Unable to parse %s: %s", HEADER_X_P2P_ONLY, err)
			} else {
				m.P2POnly = b
			}
		case ``:
			done = true
		default:
			// Graceful ignore as specified in format definition.
		}
	}

	// Read body
	var body bytes.Buffer
	for i := 0; i < bodySize; i++ {
		if c, err := reader.ReadByte(); err != nil {
			return err
		} else {
			body.WriteByte(c)
		}
	}
	if end, err := reader.ReadString('\n'); err == io.EOF {
		// that's ok
	} else if err != nil {
		return err
	} else if end != "\r\n" {
		return errors.New(`Unexpected end of body`)
	} else if body.Len() != bodySize {
		return errors.New(fmt.Sprintf(`Expected %d bytes, got %d on body`, bodySize, body.Len()))
	}
	if body, err := BodyFromBytes(body.Bytes(), charset); err != nil {
		panic(err)
	} else {
		m.Body = body
	}

	// Read files
	for _, file := range m.Files {
		var buf bytes.Buffer
		for i := 0; i < file.dataSize; i++ {
			if c, err := reader.ReadByte(); err != nil {
				return err
			} else {
				buf.WriteByte(c)
			}
		}

		if end, err := reader.ReadString('\n'); err != nil {
			return err
		} else if end != "\r\n" {
			return errors.New(`Unexpected end of attachment`)
		} else if buf.Len() != file.dataSize {
			return errors.New(fmt.Sprintf(`Expected %d bytes, got %d on %s`, file.dataSize, buf.Len(), file.Name))
		} else {
			file.data = buf.Bytes()
		}
	}

	return nil
}

// Writes Message to the given Writer in the Winlink Message
// format.
func (m *Message) WriteTo(w io.Writer) (n int64, err error) {
	writer := bufio.NewWriter(w)

	bodyBytes, err := m.Body.ToBytes(DefaultCharset, true)
	if err != nil {
		panic(err)
	}

	// TODO: Clean up this func
	wf := func(key, format string, a ...interface{}) {
		m, err := fmt.Fprintf(
			writer,
			"%s: %s\r\n",
			key,
			fmt.Sprintf(format, a...),
		)
		if err != nil {
			panic(err)
		}
		n += int64(m)
	}

	wb := func() {
		if m, err := fmt.Fprint(writer, "\r\n"); err != nil {
			panic(err)
		} else {
			n += int64(m)
		}
	}

	// Header
	wf(HEADER_MID, "%s", m.MID)
	wf(HEADER_DATE, "%s", m.Date.UTC().Format(DateLayout))
	wf(HEADER_TYPE, "%s", m.Type)
	wf(HEADER_FROM, "%s", m.From)
	for _, a := range m.To {
		wf(HEADER_TO, "%s", a)
	}
	for _, a := range m.Cc {
		wf(HEADER_CC, "%s", a)
	}
	wf(HEADER_SUBJECT, "%s", m.Subject)
	wf(HEADER_MBO, "%s", m.Mbo)

	contentType := mime.FormatMediaType(
		"text/plain",
		map[string]string{"charset": DefaultCharset},
	)
	wf(HEADER_CONTENT_TYPE, "%s", contentType)
	wf(HEADER_CONTENT_TRANSFER_ENCODING, "%s", DefaultTransferEncoding)
	if m.P2POnly {
		wf(HEADER_X_P2P_ONLY, "%t", m.P2POnly)
	}

	wf(HEADER_BODY, "%d", len(bodyBytes))
	for _, f := range m.Files {
		wf(HEADER_FILE, "%d %s", f.dataSize, f.name)
	}

	wb() // end of headers

	// Body
	if i, err := writer.Write(bodyBytes); err != nil {
		return n + int64(i), err
	} else if i != len(bodyBytes) {
		return n + int64(i), errors.New(`Body was not fully written`)
	} else {
		n += int64(i)
	}

	if len(m.Files) > 0 {
		wb() // end of body
	}

	// Files
	for _, f := range m.Files {
		if i, err := writer.Write(f.data); err != nil {
			return n + int64(i), err
		} else if i != f.dataSize {
			return n + int64(i), errors.New(`File was not fully written`)
		} else {
			n += int64(i)
		}

		wb() // end of file
	}

	writer.Flush()
	return
}

func parseHeaderLine(line string) (key, value string) {
	slice := strings.SplitN(line, `:`, 2)
	if len(slice) < 2 {
		return ``, ``
	}

	return strings.TrimSpace(slice[0]), strings.TrimSpace(slice[1])
}

func lc(str string) string { return strings.ToLower(str) }

func readHeader(line, key string) (string, error) {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(strings.ToLower(line), strings.ToLower(key)) {
		return line[len(key):], nil
	} else {
		return line, errors.New(`Expected ` + key)
	}
}

func (m *Message) String() string {
	buf := bytes.NewBufferString(``)
	w := bufio.NewWriter(buf)

	fmt.Fprintln(w, "MID:", m.MID)
	fmt.Fprintln(w, `Date:`, m.Date)
	fmt.Fprintln(w, `From:`, m.From)
	for _, to := range m.To {
		fmt.Fprintln(w, `To:`, to)
	}
	for _, cc := range m.Cc {
		fmt.Fprintln(w, `Cc:`, cc)
	}
	fmt.Fprintln(w, `Subject:`, m.Subject)

	fmt.Fprintf(w, "\n%s\n", m.Body)

	fmt.Fprintln(w, "Attachments:")
	for _, f := range m.Files {
		fmt.Fprintf(w, "\t%s [%d bytes]\n", f.Name(), f.dataSize)
	}

	w.Flush()
	return string(buf.Bytes())
}

// JSON marshaller for File
func (f *File) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(struct {
		Name string
		Size int
	}{f.Name(), f.Size()})
	return b, err
}

// The name of this attachment
func (f *File) Name() string {
	return f.name
}

// The size in bytes
func (f *File) Size() int {
	return f.dataSize
}

// Returns the attached data
func (f *File) Data() []byte {
	cpy := make([]byte, len(f.data))
	copy(cpy, f.data)
	return cpy
}

// Create a new file (attachment) with the given
// name and data
func NewFile(name string, data []byte) *File {
	return &File{
		data:     data,
		name:     name,
		dataSize: len(data),
	}
}

// Textual representation of Address
func (a Address) String() string {
	if a.Proto == "" {
		return a.Addr
	} else {
		return fmt.Sprintf("%s:%s", a.Proto, a.Addr)
	}
}

func (a Address) IsZero() bool {
	return len(a.Addr) == 0
}

func (a Address) EqualString(b string) bool {
	return a == AddressFromString(b)
}

// Function that constructs a proper Address from a string
//
// Supported formats: foo@bar.baz (SMTP proto),
// N0CALL (short winlink address) or N0CALL@winlink.org (full winlink address)
//
func AddressFromString(addr string) Address {
	if parts := strings.Split(addr, ":"); len(parts) == 2 {
		return Address{Proto: parts[0], Addr: parts[1]}
	}
	if parts := strings.Split(addr, "@"); len(parts) == 1 {
		return Address{Addr: addr}
	} else if strings.EqualFold(parts[1], "winlink.org") {
		return Address{Addr: parts[0]}
	} else {
		return Address{Proto: "SMTP", Addr: addr}
	}
}
