// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package simnode

import (
	"fmt"
	"runtime/debug"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/version"
)

type BuildInfo struct {
	Version string
	Sum     string
	Nomad   *NomadBuildInfo
}

type NomadBuildInfo struct {
	Version string
}

func GenerateBuildInfo(logger hclog.Logger) (*BuildInfo, error) {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return nil, fmt.Errorf("failed to read build info")
	}
	return &BuildInfo{
		Version: bi.Main.Version,
		Sum:     bi.Main.Sum,
		Nomad: &NomadBuildInfo{
			Version: version.GetVersion().VersionNumber(),
		},
	}, nil
}
