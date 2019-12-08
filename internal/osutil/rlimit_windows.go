// +build windows

package osutil

import "fmt"

func RaiseOpenFileLimit(max uint64) error {
	return fmt.Errorf("Not available for Windows")
}
