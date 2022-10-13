# nomad-nodesim

Nomad Client Node Simulator

To install and use you must have Go 1.19+ and run:

```
go install -v  github.com/schmichael/nomad-nodesim@latest

nomad-nodesim -help
```

See `nodesim.nomad` for an example jobspec for running nodesim in a Nomad
cluster using Docker.

See `terraform` for some of the worst Packer and Terraform code you have ever
seen. Should provision a working Nomad cluster (in AWS us-west-2) to run
`nodesim.nomad`.

## Why?

To simulate clusters orders of magnitude larger than the number of physical
machines.

Also a useful exploration of how pluggable the client is.

## What Works

- Hundreds of clients per process.
- Client restarting.

## What Could Work

Features that aren't support today but could be:

- Server mode
- Task Driver Plugins (or a builtin fake one)
- Configuration file instead of args
- Add universal "?node_id=..." HTTP API support upstream to route the RPCs to
  individual Client instances.

## What Won't Work

Features that probably can't be supported:

- Consul - Consul assumes a Consul agent per Nomad agent per Machine.
