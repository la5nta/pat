package hamlib

import "fmt"

// RigModel is the hamlib ID identifying a spesific tranceiver model.
type RigModel int

// Rig represents a receiver or tranceiver.
//
// It holds the data connection to the device.
type Rig interface {
	// Closes the connection to the Rig.
	Close() error

	// Returns the Rig's active VFO (for control).
	CurrentVFO() VFO
}

// VFO (Variable Frequency Oscillator) represents a tunable channel, from the radio operator's view.
//
// Also referred to as "BAND" (A-band/B-band) by some radio manufacturers.
type VFO interface {
	// Gets the dial frequency for this VFO.
	GetFreq() (int, error)

	// Sets the dial frequency for this VFO.
	SetFreq(f int) error

	// GetPTT returns the PTT state for this VFO.
	GetPTT() (bool, error)

	// Enable (or disable) PTT on this VFO.
	SetPTT(on bool) error
}

func Open(network, address string) (Rig, error) {
	switch network {
	case "tcp":
		return OpenTCP(address)
	case "serial":
		return OpenSerialURI(address)
	default:
		return nil, fmt.Errorf("Unknown network")
	}
}
