// +build go1.10

package main

import "mime/multipart"

func isEmptyFormFile(f *multipart.FileHeader) bool {
	return f.Size == 0 && f.Filename == ""
}
