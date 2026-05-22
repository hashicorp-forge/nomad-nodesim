# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

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
server_addr      = ["localhost:4647"]
node_num         = 10

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
