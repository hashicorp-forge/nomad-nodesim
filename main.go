package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/client"
	"github.com/hashicorp/nomad/client/config"
	"github.com/hashicorp/nomad/client/consul"
	"github.com/hashicorp/nomad/client/state"
	"github.com/hashicorp/nomad/helper/pluginutils/singleton"
	"github.com/hashicorp/nomad/helper/pointer"
	"github.com/hashicorp/nomad/helper/uuid"
	"github.com/hashicorp/nomad/nomad/structs"
	structsc "github.com/hashicorp/nomad/nomad/structs/config"
	"github.com/hashicorp/nomad/version"
	"github.com/schmichael/nomad-nodesim/allocrunnersim"
	"github.com/schmichael/nomad-nodesim/pluginsim"
	"github.com/schmichael/nomad-nodesim/simconsul"
	"github.com/schmichael/nomad-nodesim/simnode"
	"golang.org/x/exp/slog"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	workDir := ""
	defaultWorkDir := "nomad-nodesim-[pid]"
	flag.StringVar(&workDir, "dir", defaultWorkDir, "working directory")

	num := 1
	flag.IntVar(&num, "num", num, "number of client nodes")

	serverAddr := ""
	flag.StringVar(&serverAddr, "server", "", "address of server's rpc port")

	uniq := ""
	defaultUniq := "[8 hex chars]"
	flag.StringVar(&uniq, "id", defaultUniq, "nodes will be named node-[uniq]-[i]")

	flag.Parse()

	handler := slog.NewTextHandler(os.Stdout, nil)
	logger := slog.New(handler)

	if workDir == defaultWorkDir {
		workDir = fmt.Sprintf("nomad-nodesim-%d", os.Getpid())
	}

	if uniq == defaultUniq {
		uniq = uuid.Short()
	}

	logger.Info("config", "dir", workDir, "num", num, "server", serverAddr, "id", uniq)

	if ctx.Err() != nil {
		fmt.Fprintf(os.Stderr, "canceled before clients created")
		os.Exit(2)
	}

	var err error
	handles := make([]*simnode.Node, num)
	for i := 0; i < num; i++ {
		id := fmt.Sprintf("%s-%d", uniq, i)
		handles[i], err = startClient(id, logger, workDir, serverAddr)
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
	logger.Info("done")
}

func startClient(id string, logger *slog.Logger, parentDir, server string) (*simnode.Node, error) {
	nodeID := fmt.Sprintf("node-%s", id)
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
	if server != "" {
		cfg.Servers = []string{server}
	} else {
		cfg.Servers = []string{}
	}

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return nil, fmt.Errorf("unable to read build info")
	}

	tlsConfig := tlsConfigFromEnv()
	tlsEnabled := true
	if tlsConfig == nil {
		tlsConfig = &structsc.TLSConfig{}
		tlsEnabled = false
	}

	//TODO
	cfg.Node = &structs.Node{
		ID:         nodeID,
		SecretID:   nodeID + "secret", //lol
		Datacenter: "dc1",             //TODO expose option?
		Name:       nodeID,
		HTTPAddr:   "127.0.0.1:4646", // is this used? -- yes in the UI!
		TLSEnabled: tlsEnabled,
		Attributes: map[string]string{}, //TODO expose option? fake linux?
		NodeResources: &structs.NodeResources{
			Cpu: structs.LegacyNodeCpuResources{
				CpuShares:          int64(cfg.CpuCompute),
				ReservableCpuCores: []uint16{},
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
			"simnode_id":      id,
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
	cfg.ConsulConfigs = map[string]*structsc.ConsulConfig{structs.ConsulDefaultCluster: structsc.DefaultConsulConfig()}
	cfg.VaultConfigs = map[string]*structsc.VaultConfig{structs.VaultDefaultCluster: {Enabled: pointer.Of(false)}}
	cfg.StatsCollectionInterval = 10 * time.Second
	cfg.TLSConfig = tlsConfig
	cfg.GCInterval = time.Hour
	cfg.GCParallelDestroys = 1
	cfg.GCDiskUsageThreshold = 100.0
	cfg.GCInodeUsageThreshold = 100.0
	cfg.GCMaxAllocs = 10_000
	cfg.NoHostUUID = true
	cfg.ACLEnabled = false //TODO expose option
	cfg.ACLTokenTTL = time.Hour
	cfg.ACLPolicyTTL = time.Hour
	cfg.DisableRemoteExec = true
	cfg.RPCHoldTimeout = 5 * time.Second

	pluginLoader := pluginsim.New(cfg.Logger, "loader")
	cfg.PluginLoader = pluginLoader
	cfg.PluginSingletonLoader = singleton.NewSingletonLoader(cfg.Logger, pluginLoader)

	cfg.StateDBFactory = state.GetStateDBFactory(false) // store state!
	cfg.NomadServiceDiscovery = true
	//TODO TemplateDialer: could proxy to the server's address?
	cfg.Artifact = &config.ArtifactConfig{
		HTTPReadTimeout: 5 * time.Second,
		GCSTimeout:      5 * time.Second,
		GitTimeout:      5 * time.Second,
		HgTimeout:       5 * time.Second,
		S3Timeout:       5 * time.Second,
	}

	cfg.Node.Canonicalize()
	cfg.AllocRunnerFactory = allocrunnersim.NewEmptyAllocRunnerFunc

	// Consul support is disabled
	capi := simconsul.NoopCatalogAPI{}
	consulProxies := map[string]simconsul.NoopSupportedProxiesAPI{}
	cproxiesFn := func(cluster string) consul.SupportedProxiesAPI { return consulProxies[cluster] }
	serviceReg := simconsul.NoopServiceRegHandler{}

	c, err := client.NewClient(cfg, capi, cproxiesFn, serviceReg, nil)
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
