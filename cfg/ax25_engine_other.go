//go:build !libax25
// +build !libax25

package cfg

func DefaultAX25Engine() AX25Engine { return AX25EngineAGWPE }
