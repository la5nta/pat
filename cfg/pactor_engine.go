package cfg

import (
	"encoding/json"
	"fmt"
)

const (
	PactorEngineSerial PactorEngine = "serial"
	PactorEnginePTB    PactorEngine = "ptb"
)

// DefaultPactorEngine is the engine used when none is configured.
//
// It defaults to the serial (ptc-go) driver for backwards compatibility with
// existing pactor configurations.
func DefaultPactorEngine() PactorEngine { return PactorEngineSerial }

type PactorEngine string

func (p *PactorEngine) UnmarshalJSON(b []byte) error {
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return err
	}
	switch v := PactorEngine(str); v {
	case PactorEngineSerial, PactorEnginePTB:
		*p = v
		return nil
	default:
		return fmt.Errorf("invalid pactor engine '%s'", v)
	}
}
