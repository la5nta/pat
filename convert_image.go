// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"mime"
	"path"
	"strings"

	"github.com/nfnt/resize"
)

func isConvertableImageMediaType(filename, contentType string) bool {
	var mediaType string
	if contentType != "" {
		mediaType, _, _ = mime.ParseMediaType(contentType)
	}
	if mediaType == "" {
		mediaType = mime.TypeByExtension(path.Ext(filename))
	}

	switch mediaType {
	case "image/svg+xml":
		// This is a text file
		return false
	default:
		return strings.HasPrefix(mediaType, "image/")
	}
}

func convertImage(orig []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(orig))
	if err != nil {
		return nil, err
	}

	// Scale down
	if img.Bounds().Dx() > 600 {
		img = resize.Resize(600, 0, img, resize.NearestNeighbor)
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
