package cfg

import (
	"encoding/json"
	"fmt"
)

const (
	// PactorEngineSerial uses the serial-attached PACTOR TNC driver (ptc-go).
	PactorEngineSerial PactorEngine = "serial"
	// PactorEnginePTB uses the PACTOR-TCP-Bridge (PTB) driver.
	PactorEnginePTB PactorEngine = "ptb"
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
	case "":
		*p = DefaultPactorEngine()
		return nil
	case PactorEngineSerial, PactorEnginePTB:
		*p = v
		return nil
	default:
		return fmt.Errorf("invalid pactor engine '%s'", v)
	}
}