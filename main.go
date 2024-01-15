// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp-forge/nomad-nodesim/allocrunnersim"
	internalConfig "github.com/hashicorp-forge/nomad-nodesim/internal/config"
	internalSimnode "github.com/hashicorp-forge/nomad-nodesim/internal/simnode"
	"github.com/hashicorp-forge/nomad-nodesim/pluginsim"
	"github.com/hashicorp-forge/nomad-nodesim/simconsul"
	"github.com/hashicorp-forge/nomad-nodesim/simnode"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/client"
	"github.com/hashicorp/nomad/client/config"
	"github.com/hashicorp/nomad/client/consul"
	"github.com/hashicorp/nomad/client/state"
	"github.com/hashicorp/nomad/helper/pluginutils/singleton"
	"github.com/hashicorp/nomad/helper/pointer"
	"github.com/hashicorp/nomad/nomad/structs"
	structsc "github.com/hashicorp/nomad/nomad/structs/config"
	"github.com/hashicorp/nomad/version"
	"golang.org/x/exp/slog"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var flagConfig internalConfig.Config

	flag.StringVar(&flagConfig.WorkDir, "work-dir", "", "working directory")
	flag.StringVar(&flagConfig.ServerAddr, "server-addr", "", "address of server's rpc port")
	flag.StringVar(&flagConfig.NodeNamePrefix, "node-name-prefix", "", "nodes will be named [prefix]-[i]")
	flag.IntVar(&flagConfig.NodeNum, "node-num", 0, "number of client nodes")

	var configFile string
	flag.StringVar(&configFile, "config", "", "path to a config file to load")

	flag.Parse()

	// Instantiate our initial default config. This will be used to overlay all
	// other configs, starting with any supplied config file, then the CLI
	// flags.
	mergedConfig := internalConfig.Default()

	if configFile != "" {
		parsedConfigFile, err := internalConfig.ParseFile(configFile)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed parse config file: %s", err)
			os.Exit(2)
		}
		mergedConfig = mergedConfig.Merge(parsedConfigFile)
	}

	mergedConfig = mergedConfig.Merge(&flagConfig)

	handler := slog.NewTextHandler(os.Stdout, nil)
	logger := slog.New(handler)

	logger.Info("config",
		"dir", mergedConfig.WorkDir, "num", mergedConfig.NodeNum, "server", mergedConfig.ServerAddr,
		"id", mergedConfig.NodeNamePrefix)

	if ctx.Err() != nil {
		fmt.Fprintf(os.Stderr, "canceled before clients created")
		os.Exit(2)
	}

	buildInfo, err := internalSimnode.GenerateBuildInfo(logger)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to generate build info: %w", err)
		os.Exit(2)
	}

	handles := make([]*simnode.Node, mergedConfig.NodeNum)
	for i := 0; i < mergedConfig.NodeNum; i++ {
		handles[i], err = startClient(logger, buildInfo, mergedConfig, i)
		if err != nil {
			// Startup errors are fatal
			err = fmt.Errorf("error creating client %d: %w", i, err)
			break
		}
		logger.Info("started", "i", i, "total", mergedConfig.NodeNum)
		if err = ctx.Err(); err != nil {
			break
		}
	}

	if err != nil {
		logger.Error("error creating clients. cleaning up", err)
		wg := &sync.WaitGroup{}
		for _, h := range handles {
			if h == nil {
				continue
			}
			wg.Add(1)
			go func(n *simnode.Node) {
				defer wg.Done()
				if err := n.Shutdown(); err != nil {
					logger.Warn("error shutting down cliet node", "err", err, "node", n.Client.NodeID())
				}
			}(h)
		}
		wg.Wait()
		logger.Info("done cleaning up client nodes")
		os.Exit(10)
	}

	logger.Info("clients started")

	<-ctx.Done()
	logger.Info("interrupted; shutting down clients")
	wg := &sync.WaitGroup{}
	for _, h := range handles {
		wg.Add(1)
		go func(n *simnode.Node) {
			defer wg.Done()
			if err := n.Shutdown(); err != nil {
				logger.Warn("error shutting down cliet node", "err", err, "node", n.Client.NodeID())
			}
		}(h)
	}
	wg.Wait()
	logger.Info("done")
}

func startClient(logger *slog.Logger, buildInfo *internalSimnode.BuildInfo, cfg *internalConfig.Config, iter int) (*simnode.Node, error) {
	nodeID := fmt.Sprintf("%s-%v", cfg.NodeNamePrefix, iter)
	rootDir := filepath.Join(cfg.WorkDir, nodeID)
	if err := os.MkdirAll(rootDir, 0750); err != nil {
		return nil, fmt.Errorf("error creating client dir: %w", err)
	}
	logout, err := os.Create(filepath.Join(rootDir, "client.log"))
	if err != nil {
		return nil, fmt.Errorf("error creating log output: %w", err)
	}

	clientCfg := config.DefaultConfig()
	clientCfg.DevMode = false
	clientCfg.EnableDebug = true
	clientCfg.StateDir = filepath.Join(rootDir, "state")
	clientCfg.AllocDir = filepath.Join(rootDir, "allocs")

	hclogopts := &hclog.LoggerOptions{
		Name:            nodeID,
		Level:           hclog.Trace,
		Output:          logout,
		JSONFormat:      false, //TODO expose option?
		IncludeLocation: true,
		TimeFn:          time.Now,
		TimeFormat:      "2006-01-02T15:04:05Z07:00.000", //TODO expose option?
		Color:           hclog.ColorOff,
	}
	clientCfg.Logger = hclog.NewInterceptLogger(hclogopts)
	clientCfg.Region = cfg.Node.Region
	//TODO cfg.NetworkInterface

	// Fake resources
	clientCfg.NetworkSpeed = 1_000
	clientCfg.CpuCompute = 10_000
	clientCfg.MemoryMB = 10_000

	clientCfg.MaxKillTimeout = time.Minute

	//FIXME inject servers?
	if cfg.ServerAddr != "" {
		clientCfg.Servers = []string{cfg.ServerAddr}
	} else {
		clientCfg.Servers = []string{}
	}

	tlsConfig := tlsConfigFromEnv()
	tlsEnabled := true
	if tlsConfig == nil {
		tlsConfig = &structsc.TLSConfig{}
		tlsEnabled = false
	}

	//TODO
	clientCfg.Node = &structs.Node{
		ID:         nodeID,
		SecretID:   nodeID + "secret", //lol
		Datacenter: cfg.Node.Datacenter,
		Name:       nodeID,
		NodePool:   cfg.Node.NodePool,
		HTTPAddr:   "127.0.0.1:4646", // is this used? -- yes in the UI!
		TLSEnabled: tlsEnabled,
		Attributes: map[string]string{}, //TODO expose option? fake linux?
		NodeResources: &structs.NodeResources{
			Cpu: structs.LegacyNodeCpuResources{
				CpuShares:          int64(clientCfg.CpuCompute),
				ReservableCpuCores: []uint16{},
			},
			Memory:  structs.NodeMemoryResources{MemoryMB: int64(clientCfg.MemoryMB)},
			Disk:    structs.NodeDiskResources{DiskMB: 1_000_000},
			Devices: []*structs.NodeDeviceResource{},
			NodeNetworks: []*structs.NodeNetworkResource{
				&structs.NodeNetworkResource{
					Mode:       "host",
					Device:     "eth0",
					MacAddress: "d4:fb:6a:7c:31:b4",
					Speed:      1000,
					Addresses: []structs.NodeNetworkAddress{
						{
							Family:        structs.NodeNetworkAF_IPv4,
							Alias:         "public",
							Address:       "127.0.0.1", //TODO ¯\_(ツ)_/¯
							ReservedPorts: "1-1024",    // ¯\_(ツ)_/¯
							Gateway:       "127.0.0.1", // ¯\_(ツ)_/¯
						},
					},
				},
			},
			Networks:       []*structs.NetworkResource{}, // can I get away with this being empty?
			MinDynamicPort: 2000,
			MaxDynamicPort: 3000,
		},
		ReservedResources: &structs.NodeReservedResources{},
		// Resources is deprecated
		// Reserved is deprecated
		//FIXME but still used by GCConfig! Fix that in Nomad
		Reserved: &structs.Resources{},
		Links:    map[string]string{},
		Meta: map[string]string{
			"simnode_id":      cfg.NodeNamePrefix,
			"simnode_enabled": "true",
			"simnode_version": buildInfo.Version,
			"simnode_sum":     buildInfo.Sum,
		},
		//TODO NodeClass expose option?
		CSIControllerPlugins: make(map[string]*structs.CSIInfo),
		CSINodePlugins:       make(map[string]*structs.CSIInfo),
		HostVolumes:          make(map[string]*structs.ClientHostVolumeConfig),
		HostNetworks:         make(map[string]*structs.ClientHostNetworkConfig),
	}
	clientCfg.ClientMinPort = 3001  // ¯\_(ツ)_/¯
	clientCfg.ClientMaxPort = 4000  // ¯\_(ツ)_/¯
	clientCfg.MinDynamicPort = 5001 // ¯\_(ツ)_/¯
	clientCfg.MaxDynamicPort = 6000 // ¯\_(ツ)_/¯
	clientCfg.ChrootEnv = map[string]string{}
	clientCfg.Options = cfg.Node.Options
	clientCfg.Version = &version.VersionInfo{
		Version: buildInfo.Nomad.Version,
	}
	clientCfg.ConsulConfigs = map[string]*structsc.ConsulConfig{structs.ConsulDefaultCluster: structsc.DefaultConsulConfig()}
	clientCfg.VaultConfigs = map[string]*structsc.VaultConfig{structs.VaultDefaultCluster: {Enabled: pointer.Of(false)}}
	clientCfg.StatsCollectionInterval = 10 * time.Second
	clientCfg.TLSConfig = tlsConfig
	clientCfg.GCInterval = time.Hour
	clientCfg.GCParallelDestroys = 1
	clientCfg.GCDiskUsageThreshold = 100.0
	clientCfg.GCInodeUsageThreshold = 100.0
	clientCfg.GCMaxAllocs = 10_000
	clientCfg.NoHostUUID = true
	clientCfg.ACLEnabled = false //TODO expose option
	clientCfg.ACLTokenTTL = time.Hour
	clientCfg.ACLPolicyTTL = time.Hour
	clientCfg.DisableRemoteExec = true
	clientCfg.RPCHoldTimeout = 5 * time.Second

	pluginLoader := pluginsim.New(clientCfg.Logger, "loader")
	clientCfg.PluginLoader = pluginLoader
	clientCfg.PluginSingletonLoader = singleton.NewSingletonLoader(clientCfg.Logger, pluginLoader)

	clientCfg.StateDBFactory = state.GetStateDBFactory(false) // store state!
	clientCfg.NomadServiceDiscovery = true
	//TODO TemplateDialer: could proxy to the server's address?
	clientCfg.Artifact = &config.ArtifactConfig{
		HTTPReadTimeout: 5 * time.Second,
		GCSTimeout:      5 * time.Second,
		GitTimeout:      5 * time.Second,
		HgTimeout:       5 * time.Second,
		S3Timeout:       5 * time.Second,
	}

	clientCfg.Node.Canonicalize()
	clientCfg.AllocRunnerFactory = allocrunnersim.NewEmptyAllocRunnerFunc

	// Consul support is disabled
	capi := simconsul.NoopCatalogAPI{}
	consulProxies := map[string]simconsul.NoopSupportedProxiesAPI{}
	cproxiesFn := func(cluster string) consul.SupportedProxiesAPI { return consulProxies[cluster] }
	serviceReg := simconsul.NoopServiceRegHandler{}

	c, err := client.NewClient(clientCfg, capi, cproxiesFn, serviceReg, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating client: %w", err)
	}

	return simnode.New(c, logger), nil
}

func tlsConfigFromEnv() *structsc.TLSConfig {

	caCertFile := os.Getenv("NOMAD_CACERT")
	certFile := os.Getenv("NOMAD_CLIENT_CERT")
	keyFile := os.Getenv("NOMAD_CLIENT_KEY")

	if certFile == "" || caCertFile == "" || keyFile == "" {
		return nil
	}

	return &structsc.TLSConfig{
		EnableHTTP:           true,
		EnableRPC:            true,
		VerifyServerHostname: true,
		VerifyHTTPSClient:    true,
		CAFile:               caCertFile,
		CertFile:             certFile,
		KeyFile:              keyFile,
	}
}
