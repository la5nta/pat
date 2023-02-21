package cfg

import (
	"encoding/json"
	"fmt"
)

const (
	AX25EngineAGWPE     AX25Engine = "agwpe"
	AX25EngineLinux                = "linux"
	AX25EngineSerialTNC            = "serial-tnc"
)

type AX25Engine string

func (a *AX25Engine) UnmarshalJSON(p []byte) error {
	var str string
	if err := json.Unmarshal(p, &str); err != nil {
		return err
	}
	switch v := AX25Engine(str); v {
	case AX25EngineLinux, AX25EngineAGWPE, AX25EngineSerialTNC:
		*a = v
		return nil
	default:
		return fmt.Errorf("invalid AX.25 engine '%s'", v)
	}
}
