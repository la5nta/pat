// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package fbb

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/la5nta/wl2k-go/fbb/compat/mime"
)

// Representation of a receiver/sender address.
type Address struct {
	Proto string
	Addr  string
}

// File represents an attachment.
type File struct {
	data []byte
	name string
	err  error
}

// Message represent the Winlink 2000 Message Structure as defined in http://winlink.org/B2F.
type Message struct {
	// The header names are case-insensitive.
	//
	// Users should normally access common header fields
	// using the appropriate Message methods.
	Header Header

	body  []byte
	files []*File
}

type MsgType string

const (
	Private        MsgType = "Private"
	Service                = "Service"
	Inquiry                = "Inquiry"
	PositionReport         = "Position Report"
	Option                 = "Option"
	System                 = "System"
)

// NewMessage initializes and returns a new message with Type, Mbo, From and Date set.
//
// If the message type t is empty, it defaults to Private.
func NewMessage(t MsgType, mycall string) *Message {
	msg := &Message{
		Header: make(Header),
	}

	msg.Header.Set(HEADER_MID, GenerateMid(mycall))

	msg.SetDate(time.Now())
	msg.SetFrom(mycall)
	msg.Header.Set(HEADER_MBO, mycall)

	if t == "" {
		t = Private
	}
	msg.Header.Set(HEADER_TYPE, string(t))

	return msg
}

// MID returns the unique identifier of this message across the winlink system.
func (m *Message) MID() string { return m.Header.Get(HEADER_MID) }

// SetSubject sets this message's subject field.
//
// The Winlink Message Format only allow ASCII characters. Words containing non-ASCII characters are Q-encoded with DefaultCharset (as defined by RFC 2047).
func (m *Message) SetSubject(str string) {
	encoded, _ := toCharset(DefaultCharset, str)
	encoded = mime.QEncoding.Encode(DefaultCharset, encoded)

	m.Header.Set(HEADER_SUBJECT, encoded)
}

// Subject returns this message's subject header decoded using WordDecoder.
func (m *Message) Subject() string {
	str, _ := new(WordDecoder).DecodeHeader(m.Header.Get(HEADER_SUBJECT))
	return str
}

// Type returns the message type.
//
// See MsgType consts for details.
func (m *Message) Type() MsgType { return MsgType(m.Header.Get(HEADER_TYPE)) }

// Mbo returns the mailbox operator origin of this message.
func (m *Message) Mbo() string { return m.Header.Get(HEADER_MBO) }

// Body returns this message's body encoded as utf8.
func (m *Message) Body() (string, error) { return BodyFromBytes(m.body, m.Charset()) }

// Files returns the message attachments.
func (m *Message) Files() []*File { return m.files }

// SetFrom sets the From header field.
//
// SMTP: prefix is automatically added if needed, see AddressFromString.
func (m *Message) SetFrom(addr string) { m.Header.Set(HEADER_FROM, AddressFromString(addr).String()) }

// From returns the From header field as an Address.
func (m *Message) From() Address { return AddressFromString(m.Header.Get(HEADER_FROM)) }

// Set date sets the Date header field.
//
// The field is set in the format DateLayout, UTC.
func (m *Message) SetDate(t time.Time) { m.Header.Set(HEADER_DATE, t.UTC().Format(DateLayout)) }

// Date parses the Date header field according to the winlink format.
//
// Parse errors are omitted, but it's checked at serialization.
func (m *Message) Date() time.Time {
	date, _ := ParseDate(m.Header.Get(HEADER_DATE))
	return date
}

// SetBodyWithCharset translates and sets the body according to given charset.
//
// Header field Content-Transfer-Encoding is set to DefaultTransferEncoding.
// Header field Content-Type is set according to charset.
// All lines are modified to ensure CRLF.
//
// Use SetBody to use default character encoding.
func (m *Message) SetBodyWithCharset(charset, body string) error {
	m.Header.Set(HEADER_CONTENT_TRANSFER_ENCODING, DefaultTransferEncoding)
	m.Header.Set(HEADER_CONTENT_TYPE, mime.FormatMediaType(
		"text/plain",
		map[string]string{"charset": DefaultCharset},
	))

	bytes, err := StringToBody(body, DefaultCharset)
	if err != nil {
		return err
	}

	m.body = bytes
	m.Header.Set(HEADER_BODY, fmt.Sprintf("%d", len(bytes)))
	return nil
}

// SetBody sets the given string as message body using DefaultCharset.
//
// See SetBodyWithCharset for more info.
func (m *Message) SetBody(body string) error {
	return m.SetBodyWithCharset(DefaultCharset, body)
}

// BodySize returns the expected size of the body (in bytes) as defined in the header.
func (m *Message) BodySize() int { size, _ := strconv.Atoi(m.Header.Get(HEADER_BODY)); return size }

// Charset returns the body character encoding as defined in the ContentType header field.
//
// If the header field is unset, DefaultCharset is returned.
func (m *Message) Charset() string {
	_, params, err := mime.ParseMediaType(m.Header.Get(HEADER_CONTENT_TYPE))
	if err != nil {
		return DefaultCharset
	}

	if v, ok := params["charset"]; ok {
		return v
	}
	return DefaultCharset
}

// AddTo adds a new receiver for this message.
//
// It adds a new To header field per given address.
// SMTP: prefix is automatically added if needed, see AddressFromString.
func (m *Message) AddTo(addr ...string) {
	for _, a := range addr {
		m.Header.Add(HEADER_TO, AddressFromString(a).String())
	}
}

// AddCc adds a new carbon copy receiver to this message.
//
// It adds a new Cc header field per given address.
// SMTP: prefix is automatically added if needed, see AddressFromString.
func (m *Message) AddCc(addr ...string) {
	for _, a := range addr {
		m.Header.Add(HEADER_CC, AddressFromString(a).String())
	}
}

// To returns primary receivers of this message.
func (m *Message) To() (to []Address) {
	for _, str := range m.Header[HEADER_TO] {
		to = append(to, AddressFromString(str))
	}
	return
}

// Cc returns the carbon copy receivers of this message.
func (m *Message) Cc() (cc []Address) {
	for _, str := range m.Header[HEADER_CC] {
		cc = append(cc, AddressFromString(str))
	}
	return
}

// Implements ReaderFrom for Message.
//
// Reads the given io.Reader and fills in values fetched from the stream.
func (m *Message) ReadFrom(r io.Reader) error {
	reader := bufio.NewReader(r)

	if h, err := textproto.NewReader(reader).ReadMIMEHeader(); err != nil {
		return err
	} else {
		m.Header = Header(h)
	}

	// Read body
	var err error
	m.body, err = readSection(reader, m.BodySize())
	if err != nil {
		return err
	}

	// Read files
	m.files = make([]*File, len(m.Header[HEADER_FILE]))
	for i, value := range m.Header[HEADER_FILE] {
		file := new(File)
		m.files[i] = file

		slice := strings.SplitN(value, ` `, 2)
		if len(slice) != 2 {
			file.err = errors.New(`Failed to parse file header. Got: ` + value)
			continue
		}

		size, _ := strconv.Atoi(slice[0])
		file.name = slice[1]

		file.data, err = readSection(reader, size)
		if err != nil {
			file.err = err
		}
	}

	// Return error if date field is not parseable
	if err == nil {
		_, err = ParseDate(m.Header.Get(HEADER_DATE))
	}

	return err
}

func readSection(reader *bufio.Reader, readN int) ([]byte, error) {
	buf := make([]byte, readN)

	var err error
	n := 0
	for n < readN {
		m, err := reader.Read(buf[n:])
		if err != nil {
			break
		}
		n += m
	}

	if err != nil {
		return buf, err
	}

	end, err := reader.ReadString('\n')
	switch {
	case n != readN:
		return buf, io.ErrUnexpectedEOF
	case err == io.EOF:
		// That's ok
	case err != nil:
		return buf, err
	case end != "\r\n":
		return buf, errors.New("Unexpected end of section")
	}
	return buf, nil
}

// Returns true if the given Address is the only receiver of this Message.
func (m *Message) IsOnlyReceiver(addr Address) bool {
	receivers := m.Receivers()
	if len(receivers) != 1 {
		return false
	}
	return receivers[0].String() == addr.String()
}

// Method for generating a proposal of the message.
func (m *Message) Proposal(code PropCode) (*Proposal, error) {
	data, err := m.Bytes()
	if err != nil {
		return nil, err
	}

	return NewProposal(m.MID(), m.Subject(), code, data), nil
}

// Receivers returns a slice of all receivers of this message.
func (m *Message) Receivers() []Address {
	to, cc := m.To(), m.Cc()
	addrs := make([]Address, 0, len(to)+len(cc))
	if len(to) > 0 {
		addrs = append(addrs, to...)
	}
	if len(cc) > 0 {
		addrs = append(addrs, cc...)
	}
	return addrs
}

// AddFile adds the given File as an attachment to m.
func (m *Message) AddFile(f *File) {
	m.files = append(m.files, f)

	// Add header
	value := fmt.Sprintf("%d %s", f.Size(), f.Name())
	m.Header[HEADER_FILE] = append(m.Header[HEADER_FILE], value)
}

// Bytes returns the message in the Winlink Message format.
func (m *Message) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := m.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Writes Message to the given Writer in the Winlink Message format.
//
// If the Date header field is not formatted correctly, an error will be returned.
func (m *Message) Write(w io.Writer) (err error) {
	// Ensure Date field is in correct format
	if _, err = ParseDate(m.Header.Get(HEADER_DATE)); err != nil {
		return
	}

	// We use a bufio.Writer to defer error handling until Flush
	writer := bufio.NewWriter(w)

	// Header
	m.Header.Write(writer)
	writer.WriteString("\r\n") // end of headers

	// Body
	writer.Write(m.body)
	if len(m.Files()) > 0 {
		writer.WriteString("\r\n") // end of body
	}

	// Files (the order must be the same as they appear in the header)
	for _, f := range m.Files() {
		writer.Write(f.data)
		writer.WriteString("\r\n") // end of file
	}

	return writer.Flush()
}

// Message stringer.
func (m *Message) String() string {
	buf := bytes.NewBufferString(``)
	w := bufio.NewWriter(buf)

	fmt.Fprintln(w, "MID: ", m.MID())
	fmt.Fprintln(w, `Date:`, m.Date())
	fmt.Fprintln(w, `From:`, m.From())
	for _, to := range m.To() {
		fmt.Fprintln(w, `To:`, to)
	}
	for _, cc := range m.Cc() {
		fmt.Fprintln(w, `Cc:`, cc)
	}
	fmt.Fprintln(w, `Subject:`, m.Subject())

	body, _ := m.Body()
	fmt.Fprintf(w, "\n%s\n", body)

	fmt.Fprintln(w, "Attachments:")
	for _, f := range m.Files() {
		fmt.Fprintf(w, "\t%s [%d bytes]\n", f.Name(), f.Size())
	}

	w.Flush()
	return string(buf.Bytes())
}

// JSON marshaller for File.
func (f *File) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(struct {
		Name string
		Size int
	}{f.Name(), f.Size()})
	return b, err
}

// Name returns the attachment's filename.
func (f *File) Name() string { return f.name }

// Size returns the attachments's size in bytes.
func (f *File) Size() int { return len(f.data) }

// Data returns a copy of the attachment content.
func (f *File) Data() []byte {
	cpy := make([]byte, len(f.data))
	copy(cpy, f.data)
	return cpy
}

// Create a new file (attachment) with the given name and data.
func NewFile(name string, data []byte) *File {
	return &File{
		data: data,
		name: name,
	}
}

// Textual representation of Address.
func (a Address) String() string {
	if a.Proto == "" {
		return a.Addr
	} else {
		return fmt.Sprintf("%s:%s", a.Proto, a.Addr)
	}
}

// IsZero reports whether the Address is unset.
func (a Address) IsZero() bool { return len(a.Addr) == 0 }

// EqualString reports whether the given address string is equal to this address.
func (a Address) EqualString(b string) bool { return a == AddressFromString(b) }

// Function that constructs a proper Address from a string.
//
// Supported formats: foo@bar.baz (SMTP proto), N0CALL (short winlink address) or N0CALL@winlink.org (full winlink address).
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

func ParseDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Time{}, nil
	}

	date, err := time.Parse(DateLayout, dateStr)
	if err != nil {
		return date, err
	}

	return date.Local(), nil
}
