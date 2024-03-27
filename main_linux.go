// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build linux

package main

import (
	"fmt"

	"github.com/hashicorp/nomad/client/lib/cgroupslib"
)

func hackSetupCgroup(name string) {
	cgroupslib.NomadCgroupParent = fmt.Sprintf("nomad-nodesim-%s.slice", name)
}
