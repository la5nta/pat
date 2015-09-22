// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

// +build cgo
// +build libhamlib

#include <hamlib/rig.h>

void setBaudRate(RIG *r, int rate) {
	r->state.rigport.parm.serial.rate = rate;
}

int add_to_list(const struct rig_caps *rc, void* f)
{
	rigListCb(rc);
	return 1;
}

void populate_rigs_list() {
	rig_load_all_backends();
	rig_list_foreach(add_to_list, 0);
}
