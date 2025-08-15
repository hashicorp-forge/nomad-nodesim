# nomad-nodesim

Nomad Client Node Simulator

To install and use you must have Go and run:

```
go install -v -tags hashicorpmetrics github.com/schmichael/nomad-nodesim@latest

nomad-nodesim -help
```

See `nodesim.nomad` for an example jobspec for running nodesim in a Nomad
cluster using Docker.

See `terraform` for some of the worst Packer and Terraform code you have ever
seen. Should provision a working Nomad cluster (in AWS us-west-2) to run
`nodesim.nomad`.

## Gotchas

- Disable limits! Otherwise you won't scale far: https://developer.hashicorp.com/nomad/docs/configuration#limits
- If Consul is running it may try to perform 1 health check per virtual node against the *real* nodes which... can get a little intense.

## Why?

To simulate clusters orders of magnitude larger than the number of physical
machines.

Also a useful exploration of how pluggable the client is.

## What Works

- Hundreds of clients per process.
- Client restarting.

## What Could Work

Features that aren't support today but could be:

- Replace `Config.RPCHandler` with a simulated network stack
- Server mode
- Add universal "?node_id=..." HTTP API support upstream to route the RPCs to
  individual Client instances.

## What Won't Work

Features that probably can't be supported:

- Consul - Consul assumes a Consul agent per Nomad agent per Machine.

## Configuration Options
The `nomad-nodesim` application supports providing configuration parameters via CLI flags and a
config file in [HCL](https://github.com/hashicorp/hcl) format.

### CLI Flags
- `alloc-runner-type` (`"sim"`) - Defines what alloc-runner `nomad-nodesim` will use. It supports
  either the basic simulated runner included within this application, or the "real" one pulled
  directly from Nomad. Supports `"sim"` or `"real"`.

- `config` (`""`) - The path to a config file to load.

- `log-include-location` (`false`) - Whether the `nomad-nodesim` logs should include file and line
  information.

- `log-json` (`false`) - Whether the `nomad-nodesim` logs should be output in JSON format.

- `log-level` (`"debug"`) - The verbosity level of the `nomad-nodesim` logs. This currently supports
  `"trace"`, `"debug"`, `"info"`, `"warn"`, and `"error"`.

- `node-name-prefix` (`""`) - The prefix used to name each Nomad client. When not supplied, this
  will be generated using a concatenation of `node-` and a generated short UUID such as
  `node-8a48e733`.

- `node-num` (`1`) - The number of Nomad clients that will be started within the `nomad-nodesim`
  application.

- `server-addr` (`["127.0.0.1:4647"]`) This flag can be supplied multiple times and specifies the
  Nomad server RPC addresses that the Nomad clients should use for registration and connectivity.

- `work-dir` (`""`) - The working directory for the application. Each nodesim client will store its
  data directory within this. When not supplied, this will be placed within the directory it is run
  from and will be prefixed with `nomad-nodesim-`.

### Config File Parameters
- `node_name_prefix` (`""`) - The prefix used to name each Nomad client. When not supplied, this
  will be generated using a concatenation of `node-` and a generated short UUID such as
  `node-8a48e733`.

- `node_num` (`1`) - The number of Nomad clients that will be started within the nomad-nodesim
  application.

- `server_addr` (`["127.0.0.1:4647"]`) Specifies a list of Nomad server RPC addresses that the Nomad
  clients should use for registration and connectivity.

- `work_dir` (`""`) - The working directory for the application. Each nodesim client will store its
  data directory within this. When not supplied, this will be placed within the directory it is run
  from and will be prefixed with `nomad-nodesim-`.

#### `log` Parameters
- `include-location` (`false`) - Whether the `nomad-nodesim` logs should include file and line
  information.

- `json` (`false`) - Whether the `nomad-nodesim` logs should be output in JSON format.

- `level` (`"debug"`) - The verbosity level of the `nomad-nodesim` logs. This currently supports
  `"trace"`, `"debug"`, `"info"`, `"warn"`, and `"error"`.

#### `node` Parameters
- `datacenter` - (`"global"`) - Specifies the data center of the `nomad-nodesim` clients.

- `node_class` - (`""`) - Specifies an arbitrary string used to logically group
  `nomad-nodesim` clients.

- `node_pool` - (`"default"`) -  Specifies the node pool in which `nomad-nodesim` clients are
  registered.

- `region` - (`"dc1"`) - Specifies the region of the `nomad-nodesim` clients.

- `resources` - (block) - The CPU and Memory configuration that will be given to the simulated
  node.

  - `cpu_compute` - (`10_000`)  - The CPU value that the simulated node will be configured with and
    will represent the total allocatable CPU of the client.

  - `memory_mb` - (`10_000`)  - The memory MB value that the simulated node will be configured with and
    will represent the total allocatable memory of the client.

- `options` - (`"map[string]string"`) - Specifies a key-value mapping of internal configuration for
  `nomad-nodesim` clients, such as for driver configuration.

#### Config File Example
The example below demonstrates setting config parameter within a configuration file.
```hcl
work_dir         = "/tmp/nomad-nodesim/"
node_name_prefix = "node-8a48e733"
server_addr      = ["127.0.0.1:4647"]
node_num         = 100

log {
  level            = "info"
  json             = true
  include_location = true
}

node {
  region     = "kent-1"
  datacenter = "fav"
  node_pool  = "default"
  node_class = "high_mem"

  options = {
    "fingerprint.denylist" = "env_aws,env_gce,env_azure,env_digitalocean"
  }
}
```
