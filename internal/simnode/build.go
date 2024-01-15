// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package simnode

import (
	"fmt"
	"runtime/debug"

	"github.com/hashicorp/go-hclog"
	goVersion "github.com/hashicorp/go-version"
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
			Version: identifyNomadDependencyVersion(logger, bi),
		},
	}, nil
}

// identifyNomadDependencyVersion takes the passed debug build information and
// provides the Nomad version dependency. If the dependency is not found, a
// default of 0.8.0 will be returned.
func identifyNomadDependencyVersion(logger hclog.Logger, debugInfo *debug.BuildInfo) string {

	// When performing client RPCs, the Nomad server uses the
	// nomad.nodeSupportsRpc to determine if the client agent is at a version
	// which supports RPC. This feature was introduced in 0.8.0, and we will
	// not be testing on versions anywhere close to this vintage. Therefore, we
	// can default the version here, to ensure these RPCs work.
	nomadVersion := "0.8.0"

	for _, dep := range debugInfo.Deps {

		if dep.Path != "github.com/hashicorp/nomad" {
			continue
		}

		logger.Info("found Nomad dependency build information",
			"path", dep.Path, "version", dep.Version, "sum", dep.Sum)

		// Parse the version in the same way the Nomad server will when
		// performing the RPC check. This is a non-terminal error as we have
		// the default version as fallback.
		nomadDepVersion, err := goVersion.NewVersion(dep.Version)
		if err != nil {
			logger.Error("failed to parse Nomad dependency build version", "error", err)
		} else {
			nomadVersion = nomadDepVersion.String()
		}
	}

	return nomadVersion
}
