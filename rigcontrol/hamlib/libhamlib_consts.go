// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// +build cgo
// +build libhamlib

package hamlib

//#include <hamlib/rig.h>
import "C"

type Mode int

const (
	NoMode  Mode = C.RIG_MODE_NONE
	AM           = C.RIG_MODE_AM
	CW           = C.RIG_MODE_CW
	USB          = C.RIG_MODE_USB
	LSB          = C.RIG_MODE_LSB
	RTTY         = C.RIG_MODE_RTTY
	FM           = C.RIG_MODE_FM
	WFM          = C.RIG_MODE_WFM
	CWR          = C.RIG_MODE_CWR
	RTTYR        = C.RIG_MODE_RTTYR
	AMS          = C.RIG_MODE_AMS
	PKTLSB       = C.RIG_MODE_PKTLSB
	PKTUSB       = C.RIG_MODE_PKTUSB
	PKTFM        = C.RIG_MODE_PKTFM
	ECSSUSB      = C.RIG_MODE_ECSSUSB
	ECSSLSB      = C.RIG_MODE_ECSSLSB
	FAX          = C.RIG_MODE_FAX
	SAM          = C.RIG_MODE_SAM
	SAL          = C.RIG_MODE_SAL
	SAH          = C.RIG_MODE_SAH
	DSB          = C.RIG_MODE_DSB
)

type PowerState int

const (
	PowerOff     PowerState = C.RIG_POWER_OFF
	PowerOn                 = C.RIG_POWER_ON
	PowerStandby            = C.RIG_POWER_STANDBY
)
