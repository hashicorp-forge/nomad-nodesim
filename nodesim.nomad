# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

variable "num_nodes" {
  type = string
  default = 10
}

variable "servers" {
  type = list(string)
  default = ["localhost:4647"]
}

job "nodesim" {
  datacenters = ["dc1"]

  group "nodesim" {

    update {
      max_parallel = 3
    }

    task "nodesim" {
      driver = "docker"

      config {
        image = "hashicorppreview/nomad-nodesim:6cebba3"

        command = "/bin/nomad-nodesim"
        args = ["-config", "/local/config.hcl"]

	# For use on a linux host
        volumes = [
          "/sys/fs/cgroup:/sys/fs/cgroup",
        ]
      }

      template {
        data = <<EOH
work_dir         = "/tmp/nomad-nodesim/"
node_name_prefix = "nodesim"
server_addr      = ${jsonencode(var.servers)}
node_num         = ${var.num_nodes}

log {
  level            = "info"
  json             = true
  include_location = true
}

node {
  datacenter = "dc1"
  node_pool  = "default"
  options = {
    "fingerprint.denylist" = "env_aws,env_gce,env_azure,env_digitalocean"
  }
}
        EOH
        destination = "/local/config.hcl"
        once = true
      }

      resources {
        cpu    = 500
        memory = 500
      }
    }
  }
}
