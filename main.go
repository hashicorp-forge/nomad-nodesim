package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/client"
	"github.com/hashicorp/nomad/client/config"
	"github.com/hashicorp/nomad/client/state"
	"github.com/hashicorp/nomad/helper/pointer"
	"github.com/hashicorp/nomad/nomad/structs"
	structsc "github.com/hashicorp/nomad/nomad/structs/config"
	"github.com/hashicorp/nomad/version"
	"github.com/schmichael/nomad-nodesim/pluginsim"
	"github.com/schmichael/nomad-nodesim/simconsul"
	"github.com/schmichael/nomad-nodesim/simnode"
	"golang.org/x/exp/slog"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	workDir := ""
	flag.StringVar(&workDir, "dir", "", "working directory")

	num := 0
	flag.IntVar(&num, "num", 0, "number of client nodes")

	flag.Parse()

	handler := slog.NewTextHandler(os.Stdout)
	logger := slog.New(handler)

	if workDir == "" {
		fmt.Fprintf(os.Stderr, "-dir must be set\n")
		flag.Usage()
		os.Exit(1)
	}

	if num == 0 {
		fmt.Fprintf(os.Stderr, "-num must be set\n")
		flag.Usage()
		os.Exit(1)
	}

	if ctx.Err() != nil {
		fmt.Fprintf(os.Stderr, "canceled before clients created")
		os.Exit(2)
	}

	var err error
	handles := make([]*simnode.Node, num)
	for i := 0; i < num; i++ {
		handles[i], err = startClient(i, workDir, logger)
		if err != nil {
			// Startup errors are fatal
			err = fmt.Errorf("error creating client %d: %w", i, err)
			break
		}
		logger.Info("started", "i", i, "total", num)
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
}

func startClient(id int, parentDir string, logger *slog.Logger) (*simnode.Node, error) {
	nodeID := fmt.Sprintf("node-%d", id)
	rootDir := filepath.Join(parentDir, nodeID)
	if err := os.MkdirAll(rootDir, 0750); err != nil {
		return nil, fmt.Errorf("error creating client dir: %w", err)
	}
	logout, err := os.Create(filepath.Join(rootDir, "client.log"))
	if err != nil {
		return nil, fmt.Errorf("error creating log output: %w", err)
	}

	cfg := config.DefaultConfig()
	cfg.DevMode = false
	cfg.EnableDebug = true
	cfg.StateDir = filepath.Join(rootDir, "state")
	cfg.AllocDir = filepath.Join(rootDir, "allocs")

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
	cfg.Logger = hclog.NewInterceptLogger(hclogopts)
	//TODO cfg.Region
	//TODO cfg.NetworkInterface

	// Fake resources
	cfg.NetworkSpeed = 1_000
	cfg.CpuCompute = 10_000
	cfg.MemoryMB = 10_000

	cfg.MaxKillTimeout = time.Minute

	//FIXME inject servers?
	cfg.Servers = []string{}

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return nil, fmt.Errorf("unable to read build info")
	}

	//TODO
	cfg.Node = &structs.Node{
		ID:         nodeID,
		SecretID:   nodeID + "secret", //lol
		Datacenter: "dc1",             //TODO expose option?
		Name:       nodeID,
		HTTPAddr:   fmt.Sprintf("http://localhost:4646/%s", nodeID), // is this used?
		TLSEnabled: false,
		Attributes: map[string]string{}, //TODO expose option? fake linux?
		NodeResources: &structs.NodeResources{
			Cpu: structs.NodeCpuResources{
				CpuShares:          int64(cfg.CpuCompute),
				TotalCpuCores:      4,
				ReservableCpuCores: []uint16{0, 1, 2, 3},
			},
			Memory:  structs.NodeMemoryResources{MemoryMB: int64(cfg.MemoryMB)},
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
			"simnode_index":   strconv.Itoa(id),
			"simnode_enabled": "true",
			"simnode_version": bi.Main.Version,
			"simnode_sum":     bi.Main.Sum,
		},
		//TODO NodeClass expose option?
		CSIControllerPlugins: make(map[string]*structs.CSIInfo),
		CSINodePlugins:       make(map[string]*structs.CSIInfo),
		HostVolumes:          make(map[string]*structs.ClientHostVolumeConfig),
		HostNetworks:         make(map[string]*structs.ClientHostNetworkConfig),
	}
	cfg.ClientMinPort = 3001  // ¯\_(ツ)_/¯
	cfg.ClientMaxPort = 4000  // ¯\_(ツ)_/¯
	cfg.MinDynamicPort = 5001 // ¯\_(ツ)_/¯
	cfg.MaxDynamicPort = 6000 // ¯\_(ツ)_/¯
	cfg.ChrootEnv = map[string]string{}
	cfg.Options = map[string]string{}
	cfg.Version = &version.VersionInfo{
		Revision: bi.Main.Sum,
		Version:  bi.Main.Version,
	}
	cfg.ConsulConfig = &structsc.ConsulConfig{}
	cfg.VaultConfig = &structsc.VaultConfig{Enabled: pointer.Of(false)}
	cfg.StatsCollectionInterval = 10 * time.Second
	cfg.TLSConfig = &structsc.TLSConfig{}
	cfg.GCInterval = time.Hour
	cfg.GCParallelDestroys = 1
	cfg.GCDiskUsageThreshold = 1.0
	cfg.GCInodeUsageThreshold = 1.0
	cfg.GCMaxAllocs = 10_000
	cfg.NoHostUUID = true
	cfg.ACLEnabled = false //TODO expose option
	cfg.ACLTokenTTL = time.Hour
	cfg.ACLPolicyTTL = time.Hour
	cfg.DisableRemoteExec = true
	cfg.RPCHoldTimeout = 5 * time.Second
	cfg.PluginLoader = pluginsim.New(logger, "loader")
	cfg.PluginSingletonLoader = pluginsim.New(logger, "singleton")
	cfg.StateDBFactory = state.GetStateDBFactory(true) // noop

	cfg.Node.Canonicalize()

	capi := simconsul.NoopCatalogAPI{}
	cproxies := simconsul.NoopSupportedProxiesAPI{}
	serviceReg := simconsul.NoopServiceRegHandler{}

	//TODO overwrite plugins and fingerprinters
	c, err := client.NewClient(cfg, capi, cproxies, serviceReg, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating client: %w", err)
	}
	return simnode.New(c, logger), nil
}
