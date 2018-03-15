// +build !go1.10

package main

import "mime/multipart"

// Prior to Go 1.10, empty form files was filtered automatically by the multipart.Parser
func isEmptyFormFile(f *multipart.FileHeader) bool { return false }
