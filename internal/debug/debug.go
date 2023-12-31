package debug

import (
	"log"
	"os"
	"strconv"
)

const (
	EnvVar = "PAT_DEBUG"
	Prefix = "[DEBUG] "
)

var enabled bool

func init() {
	enabled, _ = strconv.ParseBool(os.Getenv(EnvVar))
}

func Enabled() bool { return enabled }

func Printf(format string, v ...interface{}) {
	if !enabled {
		return
	}
	log.Printf(Prefix+format, v...)
}
