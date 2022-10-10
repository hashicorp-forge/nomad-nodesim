package pluginsim

import (
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/nomad/helper/pluginutils/loader"
	"github.com/hashicorp/nomad/plugins/base"
	"golang.org/x/exp/slog"
)

type PluginCatalog struct {
	logger *slog.Logger
	name   string
}

func New(pl *slog.Logger, name string) *PluginCatalog {
	return &PluginCatalog{
		logger: pl.With("plugincat", name),
		name:   name,
	}
}

// Dispense returns the plugin given its name and type. This will also
// configure the plugin
func (c *PluginCatalog) Dispense(name, pluginType string, config *base.AgentConfig, logger hclog.Logger) (loader.PluginInstance, error) {
	c.logger.Debug("Dispense()", "name", name, "type", pluginType, "config", config, "logger", logger, "p", c)
	panic("not implemented")
}

// Reattach is used to reattach to a previously launched external plugin.
func (c *PluginCatalog) Reattach(name, pluginType string, config *plugin.ReattachConfig) (loader.PluginInstance, error) {
	c.logger.Debug("Reattach()", "name", name, "type", pluginType, "config", config, "p", c)
	panic("not implemented")
}

// Catalog returns the catalog of all plugins keyed by plugin type
func (c *PluginCatalog) Catalog() map[string][]*base.PluginInfoResponse {
	c.logger.Debug("Catalog()", "p", c)
	return map[string][]*base.PluginInfoResponse{}
}
