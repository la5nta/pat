package forms

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"os"
	"sync"
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

type trimBomReader struct {
	r    *bufio.Reader
	once sync.Once
}

func (r *trimBomReader) Read(p []byte) (int, error) {
	r.once.Do(func() {
		peek, _ := r.r.Peek(3)
		if bytes.ContainsRune(peek, '\uFEFF') {
			r.r.Discard(3)
		}
	})
	return r.r.Read(p)
}

func newTrimBomReader(r io.Reader) io.Reader {
	return &trimBomReader{r: bufio.NewReader(r)}
}
