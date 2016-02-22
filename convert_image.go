// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"io"
	"io/ioutil"
	"mime"
	"os"
	"os/exec"
	"path"
	"strings"
)

var ErrMissingImageMagick = errors.New("Unable to find ImageMagick's convert binary.")

const convertBin = "convert"

func isImageMediaType(filename, contentType string) bool {
	var mediaType string
	if contentType != "" {
		mediaType, _, _ = mime.ParseMediaType(contentType)
	}
	if mediaType == "" {
		mediaType = mime.TypeByExtension(path.Ext(filename))
	}

	return strings.HasPrefix(mediaType, "image/")
}

// The output is a jpg file
func convertImage(rd io.Reader) ([]byte, error) {
	if _, err := exec.LookPath(convertBin); err != nil {
		return nil, ErrMissingImageMagick
	}

	//convert [filename] -quality 75 -colors 254 -resize 600x400 [filename].jpg
	oldF, err := ioutil.TempFile("", "pat_convert_")
	if err != nil {
		return nil, err
	}
	defer func() {
		oldF.Close()
		os.Remove(oldF.Name())
	}()

	if _, err := io.Copy(oldF, rd); err != nil {
		return nil, err
	}
	oldF.Sync()

	newF, err := ioutil.TempFile("", "pat_convert_")
	if err != nil {
		return nil, err
	}
	defer func() {
		newF.Close()
		os.Remove(newF.Name())
	}()

	out, err := exec.Command(convertBin, oldF.Name(), "-quality", "75", "-colors", "254", "-resize", "600x400", "jpg:"+newF.Name()).CombinedOutput()
	if err != nil {
		return nil, errors.New(string(out))
	}
	newF.Seek(0, 0)

	return ioutil.ReadAll(newF)
}
