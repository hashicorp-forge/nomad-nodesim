// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2/hclsimple"
	"github.com/hashicorp/nomad/helper/uuid"
)

// Config is the top-level and main configuration object used for running the
// nodesim application. It contains all the information required to run both
// the application and all executed Nomad simulated nodes.
type Config struct {

	// WorkDir is the filesystem path that nodesim will use to write
	// application data. This path will also be passed to each simulated client
	// for their state and allocation directories.
	WorkDir string `hcl:"work_dir,optional"`

	// NodeNamePrefix is the prefix identifier that should be used when
	// generating the simulated client name and ID.
	NodeNamePrefix string `hcl:"node_name_prefix,optional"`

	// ServerAddr is a slice of server RPC addresses which will be used for the
	// clients initial registration.
	ServerAddr arrayFlagVar `hcl:"server_addr,optional"`

	// NodeNum is the number of Nomad clients/nodes that will be started within
	// this single nodesim process. Some basic testing indicates you will need
	// to allocate 0.8MHz of CPU and 0.7MiB of memory per client instance.
	NodeNum int `hcl:"node_num,optional"`

	// AllocRunnerType defines what alloc-runner nodesim will build. It
	// supports either the basic simulated runner included within this
	// application, or the "real" one pulled directly from Nomad.
	AllocRunnerType string `hcl:"alloc_runner_type,optional"`

	Log  *Log  `hcl:"log,block"`
	Node *Node `hcl:"node,block"`
}

const (
	// AllocRunnerTypeSim is the simulated alloc runner included within nodesim
	// under the package allocrunnersim.
	AllocRunnerTypeSim = "sim"

	// AllocRunnerTypeReal is the alloc runner which is set up from the Nomad
	// codebase directly. This implements the task runner and hooks for a real
	// client experience.
	AllocRunnerTypeReal = "real"
)

// Node is the configuration object that is used to configure the Nomad clients
// that are instantiated by simnode. It contains a small subset of parameters
// which allow for useful configuration to account for environment specific
// details, are testing scenarios.
type Node struct {
	Region     string `hcl:"region,optional"`
	Datacenter string `hcl:"datacenter,optional"`
	NodePool   string `hcl:"node_pool,optional"`
	NodeClass  string `hcl:"node_class,optional"`

	// Options is a list of Nomad client options mapping as described:
	// https://developer.hashicorp.com/nomad/docs/configuration/client#options
	//
	// In particular, this can be used to disable finger-printers which are not
	// required, or which have lengthy timeouts which can slow client startup
	// times.
	Options map[string]string `hcl:"options,optional"`

	Resources *NodeResource `hcl:"resources,block"`
}

// NodeResource is the CPU and Memory configuration that will be given to the
// simulated node.
type NodeResource struct {
	CPUCompute uint64 `hcl:"cpu_compute,optional"`
	MemoryMB   uint64 `hcl:"memory_mb,optional"`
}

// Default returns a default configuration object with all parameters set to
// their default values. This returned object can be used as the basis for
// merging user supplied data.
func Default() *Config {
	return &Config{
		WorkDir:         fmt.Sprintf("nomad-nodesim-%d", os.Getpid()),
		NodeNamePrefix:  fmt.Sprintf("node-%s", uuid.Short()),
		ServerAddr:      []string{"127.0.0.1:4647"},
		NodeNum:         1,
		AllocRunnerType: AllocRunnerTypeSim,
		Log: &Log{
			Level:           "debug",
			JSON:            false,
			IncludeLocation: false,
		},
		Node: &Node{
			Region:     "global",
			Datacenter: "dc1",
			NodePool:   "default",
			NodeClass:  "",
			Options:    map[string]string{},
			Resources: &NodeResource{
				CPUCompute: 10_000,
				MemoryMB:   10_000,
			},
		},
	}
}

func (c *Config) Merge(z *Config) *Config {
	if c == nil {
		return z
	}

	result := *c

	if z.WorkDir != "" {
		result.WorkDir = z.WorkDir
	}
	if z.NodeNamePrefix != "" {
		result.NodeNamePrefix = z.NodeNamePrefix
	}
	if len(z.ServerAddr) > 0 {
		result.ServerAddr = z.ServerAddr
	}
	if z.NodeNum > 0 {
		result.NodeNum = z.NodeNum
	}
	if z.AllocRunnerType != "" {
		result.AllocRunnerType = z.AllocRunnerType
	}
	if z.Log != nil {
		result.Log = c.Log.merge(z.Log)
	}
	if z.Node != nil {
		result.Node = c.Node.merge(z.Node)
	}

	return &result
}

func (n *Node) merge(z *Node) *Node {
	if n == nil {
		return z
	}

	result := *n

	if z.Region != "" {
		result.Region = z.Region
	}
	if z.Datacenter != "" {
		result.Datacenter = z.Datacenter
	}
	if z.NodePool != "" {
		result.NodePool = z.NodePool
	}
	if z.NodeClass != "" {
		result.NodeClass = z.NodeClass
	}
	if z.Options != nil {
		for k, v := range z.Options {
			result.Options[k] = v
		}
	}
	if z.Resources != nil {
		if z.Resources.CPUCompute != 0 {
			result.Resources.CPUCompute = z.Resources.CPUCompute
		}
		if z.Resources.MemoryMB != 0 {
			result.Resources.MemoryMB = z.Resources.MemoryMB
		}
	}

	return &result
}

func ParseFile(filePath string) (*Config, error) {

	fi, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	if fi.IsDir() {
		return nil, errors.New("loading config from a directory is not supported")
	}

	cleanedFilePath := filepath.Clean(filePath)

	cfg := &Config{}

	if err := hclsimple.DecodeFile(cleanedFilePath, nil, cfg); err != nil {
		return nil, fmt.Errorf("failed to decode file: %w", err)
	}

	return cfg, nil
}
