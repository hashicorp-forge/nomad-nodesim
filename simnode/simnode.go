// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package simnode

import (
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/client"
)

type Node struct {
	Client *client.Client
	logger hclog.Logger
}

func New(c *client.Client, logger hclog.Logger) *Node {
	return &Node{
		Client: c,
		logger: logger.With("node", c.NodeID()),
	}
}

func (n *Node) Shutdown() error {
	n.logger.Debug("shuting down...")
	return n.Client.Shutdown()
}
