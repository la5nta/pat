package app

import (
	"bytes"
	"image"
	"image/jpeg"  // jpeg decoding (and encoding)
	_ "image/png" // png decoding
	"io"
	"log"
	"mime"
	"path"
	"path/filepath"
	"strings"

	"github.com/la5nta/wl2k-go/fbb"
	_ "golang.org/x/image/bmp" // bmp decoding
	"golang.org/x/image/draw"
)

func AddAttachment(msg *fbb.Message, filename string, contentType string, r io.Reader) error {
	p, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if ok, mediaType := isConvertableImageMediaType(filename, contentType); ok {
		log.Printf("Auto converting '%s' [%s]...", filename, mediaType)
		if converted, err := convertImage(p); err != nil {
			log.Printf("Error converting image: %s", err)
		} else {
			log.Printf("Done converting '%s'.", filename)
			ext := filepath.Ext(filename)
			filename = filename[:len(filename)-len(ext)] + ".jpg"
			p = converted
		}
	}
	msg.AddFile(fbb.NewFile(filename, p))
	return nil
}

func isConvertableImageMediaType(filename, contentType string) (convertable bool, mediaType string) {
	if contentType != "" {
		mediaType, _, _ = mime.ParseMediaType(contentType)
	}
	if mediaType == "" {
		mediaType = mime.TypeByExtension(path.Ext(filename))
	}

	switch mediaType {
	case "image/svg+xml":
		// This is a text file
		return false, mediaType
	default:
		return strings.HasPrefix(mediaType, "image/"), mediaType
	}
}

func convertImage(orig []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(orig))
	if err != nil {
		return nil, err
	}

	// Scale to a maximum width of 600px
	if img.Bounds().Dx() > 600 {
		img = resize(img, 600)
	}

	// Re-encode as low quality jpeg
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 40}); err != nil {
		return orig, err
	}
	if buf.Len() >= len(orig) {
		return orig, nil
	}
	return buf.Bytes(), nil
}

func resize(img image.Image, newWidth int) image.Image {
	bounds := img.Bounds()
	height := bounds.Dy() * newWidth / bounds.Dx()
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, height))
	draw.NearestNeighbor.Scale(dst, dst.Bounds(), img, bounds, draw.Src, nil)
	return dst
}
