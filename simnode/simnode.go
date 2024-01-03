// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package simnode

import (
	"github.com/hashicorp/nomad/client"
	"golang.org/x/exp/slog"
)

type Node struct {
	Client *client.Client
	logger *slog.Logger
}

func New(c *client.Client, pl *slog.Logger) *Node {
	return &Node{
		Client: c,
		logger: pl.With("node", c.NodeID()),
	}
}

func (n *Node) Shutdown() error {
	n.logger.Debug("shuting down...")
	return n.Client.Shutdown()
}
