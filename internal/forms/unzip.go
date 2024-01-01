package forms

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func unzip(srcArchivePath, dstRoot string) error {
	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(zf *zip.File) error {
		if zf.FileInfo().IsDir() {
			return nil
		}
		destPath := filepath.Join(dstRoot, zf.Name)

		// Check for ZipSlip (Directory traversal)
		if !strings.HasPrefix(destPath, filepath.Clean(dstRoot)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", destPath)
		}

		// Ensure target directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("can't create target directory: %w", err)
		}

		// Write file
		src, err := zf.Open()
		if err != nil {
			return err
		}
		defer src.Close()
		dst, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, zf.Mode())
		if err != nil {
			return err
		}
		defer dst.Close()
		_, err = io.Copy(dst, src)
		return err
	}

	r, err := zip.OpenReader(srcArchivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if err := extractAndWriteFile(f); err != nil {
			return err
		}
	}
	return r.Close()
}
