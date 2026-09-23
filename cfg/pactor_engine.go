package cfg

import (
	"encoding/json"
	"fmt"
)

// DefaultPactorEngine is the engine used when none is configured.
//
// It defaults to the serial (ptc-go) driver for backwards compatibility with
// existing pactor configurations.
func DefaultPactorEngine() PactorEngine { return "serial" }

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
	case "serial", "ptb":
		*p = v
		return nil
	default:
		return fmt.Errorf("invalid pactor engine '%s'", v)
	}
}
