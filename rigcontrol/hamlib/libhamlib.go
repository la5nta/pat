// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// +build cgo
// +build libhamlib

package hamlib

/*
#cgo LDFLAGS: -lhamlib
#include <string.h>
#include <hamlib/rig.h>

void setBaudRate(RIG *r, int rate);
int add_to_list(const struct rig_caps *rc, void* f);
void populate_rigs_list();
*/
import "C"

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

var ErrUnknownModel = errors.New("Unknown rig model")

// Rig represents a receiver or tranceiver.
//
// It holds the data connection to the device.
type SerialRig struct{ r C.RIG }

// VFO (Variable Frequency Oscillator) represents a tunable channel,
// from the radio operator's view.
//
// Also referred to as "BAND" (A-band/B-band) by some radio manufacturers.
type cVFO struct {
	v C.vfo_t
	r *SerialRig
}

var rigList []*C.struct_rig_caps

func init() {
	C.rig_set_debug(C.RIG_DEBUG_BUG)

	rigList = make([]*C.struct_rig_caps, 0, 250)
	C.populate_rigs_list()
}

//export rigListCb
func rigListCb(rc *C.struct_rig_caps) {
	rigList = append(rigList, rc)
}

// Rigs returns a map from RigModel to description (manufacturer and model)
// of all known rigs.
func Rigs() map[RigModel]string {
	list := make(map[RigModel]string, len(rigList))
	for _, rc := range rigList {
		list[RigModel(rc.rig_model)] = fmt.Sprintf("%s %s",
			C.GoString(rc.mfg_name),
			C.GoString(rc.model_name))
	}
	return list
}

// OpenSerial connects to the transceiver and returns a ready to use Rig.
//
// Caller must remember to Close the Rig after use.
func OpenSerial(model RigModel, path string, baudrate int) (*SerialRig, error) {
	rig := C.rig_init(C.rig_model_t(model))
	if rig == nil {
		return nil, ErrUnknownModel
	}

	// Set baudrate
	C.setBaudRate(rig, C.int(baudrate))

	// Set path to tty
	C.strncpy(&rig.state.rigport.pathname[0], C.CString(path), C.FILPATHLEN-1)

	err := codeToError(C.rig_open(rig))
	if err != nil {
		return nil, fmt.Errorf("Unable to open rig: %s", err)
	}

	return &SerialRig{*rig}, nil
}

// OpenSerialURI connects to the transceiver and returns a ready to use Rig.
//
// Expects a valid URI with path to a tty or COM-port.
// Additional query parameters:
//    model    (integer)
//    baudrate (integer)
// E.g. "/dev/ttyS0?model=123&baudrate=9600".
//
// Caller must remember to Close the Rig after use.
func OpenSerialURI(uri string) (*SerialRig, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("Invalid address format")
	}

	modelStr := u.Query().Get("model")
	if modelStr == "" {
		return nil, fmt.Errorf("Missing model parameter")
	}
	model, err := strconv.Atoi(modelStr)
	if err != nil {
		return nil, fmt.Errorf("Invalid model format")
	}

	baudStr := u.Query().Get("baudrate")
	if baudStr == "" {
		return nil, fmt.Errorf("Missing baudrate parameter")
	}
	baudrate, err := strconv.Atoi(baudStr)
	if err != nil {
		return nil, fmt.Errorf("Invalid baudrate format")
	}

	return OpenSerial(RigModel(model), u.Path, baudrate)
}

// Closes the connection to the Rig.
func (r *SerialRig) Close() error {
	C.rig_close(&r.r)
	return nil
}

// Returns the Rig's active VFO (for control).
func (r *SerialRig) CurrentVFO() VFO {
	return cVFO{C.RIG_VFO_CURR, r}
}

// Returns the Rig's A vfo.
func (r *SerialRig) VFOA() VFO {
	return cVFO{C.RIG_VFO_A, r}
}

// Returns the Rig's B vfo.
func (r *SerialRig) VFOB() VFO {
	return cVFO{C.RIG_VFO_B, r}
}

func (r *SerialRig) SetPowerState(pwr PowerState) error {
	return codeToError(C.rig_set_powerstat(&r.r, C.powerstat_t(pwr)))
}

// Enable (or disable) PTT on this VFO.
func (v cVFO) SetPTT(on bool) error {
	var ns C.ptt_t
	if on {
		ns = C.RIG_PTT_ON
	} else {
		ns = C.RIG_PTT_OFF
	}

	return codeToError(C.rig_set_ptt(&v.r.r, v.v, ns))
}

// GetPTT returns the PTT state for this VFO.
func (v cVFO) GetPTT() (bool, error) {
	var ptt C.ptt_t
	err := codeToError(C.rig_get_ptt(&v.r.r, v.v, &ptt))
	return ptt == C.RIG_PTT_ON, err
}

// Sets the dial frequency for this VFO.
func (v cVFO) SetFreq(freq int) error {
	return codeToError(
		C.rig_set_freq(&v.r.r, v.v, C.freq_t(freq)),
	)
}

// Gets the dial frequency for this VFO.
func (v cVFO) GetFreq() (int, error) {
	var freq C.freq_t
	err := codeToError(C.rig_get_freq(&v.r.r, v.v, &freq))
	return int(freq), err
}

// SetMode switches to the given Mode using the supplied passband bandwidth.
func (v cVFO) SetMode(m Mode, pbw int) error {
	return codeToError(C.rig_set_mode(&v.r.r, v.v,
		C.rmode_t(m),
		C.pbwidth_t(pbw),
	))
}

// GetMode returns this VFO's active Mode and passband bandwidth.
func (v cVFO) GetMode() (m Mode, pwb int, err error) {
	var cm C.rmode_t
	var cpwb C.pbwidth_t
	err = codeToError(C.rig_get_mode(&v.r.r, v.v, &cm, &cpwb))
	return Mode(cm), int(cpwb), err
}

// Returns the narrow (closest) passband for the given Mode.
func (r *SerialRig) PassbandNarrow(m Mode) int {
	return int(C.rig_passband_narrow(&r.r, C.rmode_t(m)))
}

// Returns the normal (default) passband for the given Mode.
func (r *SerialRig) PassbandNormal(m Mode) int {
	return int(C.rig_passband_normal(&r.r, C.rmode_t(m)))
}

// Returns the wide (default) passband for the given Mode.
func (r *SerialRig) PassbandWide(m Mode) int {
	return int(C.rig_passband_wide(&r.r, C.rmode_t(m)))
}

func codeToError(code C.int) error {
	if code == C.RIG_OK {
		return nil
	}
	return errors.New(C.GoString(C.rigerror(code)))
}
