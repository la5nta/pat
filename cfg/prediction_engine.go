package cfg

import (
	"encoding/json"
	"fmt"
	"strings"
)

type PredictionEngine string

const (
	PredictionEngineAuto     PredictionEngine = ""
	PredictionEngineDisabled PredictionEngine = "disabled"
	PredictionEngineVOACAP   PredictionEngine = "voacap"
)

func (p *PredictionEngine) UnmarshalJSON(b []byte) error {
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return err
	}
	switch v := PredictionEngine(strings.ToLower(strings.TrimSpace(str))); v {
	case PredictionEngineVOACAP, PredictionEngineDisabled, PredictionEngineAuto:
		*p = v
		return nil
	default:
		return fmt.Errorf("invalid prediction engine '%s'", v)
	}
}
