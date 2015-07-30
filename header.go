package wl2k

import (
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"strings"
)

// This file contains code from from net/http/header.go

// Common Winlink 2000 Message headers
const (
	HEADER_MID     = `Mid`
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

	// The default body charset seems to be ISO-8859-1
	//
	// The Winlink Message Structure docs says that the body should
	// be ASCII-only, but RMS Express seems to encode the body as
	// ISO-8859-1. This is also the charset set (Content-Type header)
	// when a message reaches an SMTP server.
	DefaultCharset = "ISO-8859-1"

	// Mails going out over SMTP from the Winlink system is sent
	// with the header 'Content-Transfer-Encoding: 7bit', but
	// let's be reasonable... we don't send ASCII-only body.
	DefaultTransferEncoding = "8bit"

	// The date (in UTC) format as described in the Winlink
	// Message Structure docs (YYYY/MM/DD HH:MM).
	DateLayout = `2006/01/02 15:04`
)

// A Header represents the key-value pairs in a Winlink 2000 Message header.
type Header map[string][]string

// Add adds the key, value pair to the header.
// It appends to any existing values associated with key.
func (h Header) Add(key, value string) {
	textproto.MIMEHeader(h).Add(key, value)
}

// Set sets the header entries associated with key to
// the single element value.  It replaces any existing
// values associated with key.
func (h Header) Set(key, value string) {
	textproto.MIMEHeader(h).Set(key, value)
}

// Get gets the first value associated with the given key.
// If there are no values associated with the key, Get returns "".
// To access multiple values of a key, access the map directly
// with CanonicalHeaderKey.
func (h Header) Get(key string) string {
	return textproto.MIMEHeader(h).Get(key)
}

// get is like Get, but key must already be in CanonicalHeaderKey form.
func (h Header) get(key string) string {
	if v := h[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// Del deletes the values associated with key.
func (h Header) Del(key string) {
	textproto.MIMEHeader(h).Del(key)
}

// Write writes a header in wire format.
func (h Header) Write(w io.Writer) error {
	var err error

	// Mid is required
	if h.get(HEADER_MID) == "" {
		return errors.New("Missing MID in header")
	}

	// Write mid, this is defined to be the first value
	_, err = fmt.Fprintf(w, "Mid: %s\r\n", h.get(HEADER_MID))
	if err != nil {
		return err
	}

	for k, slice := range h {
		if strings.EqualFold(k, HEADER_MID) {
			continue
		}

		for _, v := range slice {
			v = textproto.TrimString(v)
			_, err = fmt.Fprintf(w, "%s: %s\r\n", k, v)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
