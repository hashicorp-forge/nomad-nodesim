// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pluginsim

import (
	"fmt"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/drivers/mock"
	"github.com/hashicorp/nomad/helper/pluginutils/catalog"
	"github.com/hashicorp/nomad/helper/pluginutils/loader"
)

func New(logger hclog.Logger) *loader.PluginLoader {

	loader, err := setupPluginLoader(logger)
	if err != nil {
		panic(err)
	}
	return loader
}

// setupPlugins is used to setup the plugin loaders.
func setupPluginLoader(logger hclog.Logger) (*loader.PluginLoader, error) {
	// Get our internal plugins
	internal, err := internalPluginConfigs(logger)
	if err != nil {
		return nil, err
	}

	// Build the plugin loader
	config := &loader.PluginLoaderConfig{
		Logger:            logger,
		InternalPlugins:   internal,
		SupportedVersions: loader.AgentSupportedApiVersions,
	}
	l, err := loader.NewPluginLoader(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create plugin loader: %v", err)
	}

	for k, plugins := range l.Catalog() {
		for _, p := range plugins {
			logger.Info("detected plugin", "name", p.Name, "type", k, "plugin_version", p.PluginVersion)
		}
	}

	return l, nil
}

func internalPluginConfigs(logger hclog.Logger) (map[loader.PluginID]*loader.InternalPluginConfig, error) {

	// The simulated nodes are not expected to run any real work, but having a
	// mock driver available is very useful for testing. We therefore generate
	// our own plugin catalog, which currently just contains the mock driver
	// plugin.
	pluginCatalog := map[loader.PluginID]*catalog.Registration{
		mock.PluginID: {
			Config: mock.PluginConfig,
		},
	}

	// Create our map of plugins
	internal := make(map[loader.PluginID]*loader.InternalPluginConfig, 1)

	// Provide an empty plugin options map
	var options map[string]string

	for id, reg := range pluginCatalog {
		if reg.Config == nil {
			logger.Error("skipping loading internal plugin because it is missing its configuration", "plugin", id)
			continue
		}

		pluginConfig := reg.Config.Config
		if reg.ConfigLoader != nil {
			pc, err := reg.ConfigLoader(options)
			if err != nil {
				return nil, fmt.Errorf("failed to retrieve config for internal plugin %v: %v", id, err)
			}

			pluginConfig = pc
		}

		internal[id] = &loader.InternalPluginConfig{
			Factory: reg.Config.Factory,
			Config:  pluginConfig,
		}
	}

	return internal, nil
}
