// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package config

// Log details the log configuration parameters for the nodesim application. It
// does not control the log configuration of each simulated Nomad node.
type Log struct {

	// Level is the logger level that should be used. This can be any one of
	// "trace", "debug", "info", "warn", "error", and "off" and is
	// case-insensitive.
	Level string `hcl:"level,optional"`

	// JSON indicates whether the log lines should be formatted in JSON or not.
	JSON bool `hcl:"json,optional"`

	// IncludeLocation indicates whether to include file and line information
	// in each log line.
	IncludeLocation bool `hcl:"include_location,optional"`
}

func (l *Log) merge(z *Log) *Log {
	if l == nil {
		return z
	}

	result := *l

	if z.Level != "" {
		result.Level = z.Level
	}
	if z.JSON {
		result.JSON = z.JSON
	}
	if z.IncludeLocation {
		result.IncludeLocation = z.IncludeLocation
	}

	return &result
}
