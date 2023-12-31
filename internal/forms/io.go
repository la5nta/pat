package forms

import (
	"bytes"
	"log"
	"os"
	"unicode/utf8"
)

func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	// skipping over UTF-8 byte-ordering mark EFBBEF, some 3rd party templates use it
	// (e.g. Sonoma county's ICS213_v2.1_SonomaACS_TwoWay_Initial_Viewer.html)
	data = trimBom(data)
	if !utf8.Valid(data) {
		log.Printf("Warning: unsupported string encoding in file %q, expected UTF-8", path)
	}
	return string(data), nil
}

func trimBom(p []byte) []byte {
	return bytes.TrimLeftFunc(p, func(r rune) bool { return r == '\uFEFF' })
}
