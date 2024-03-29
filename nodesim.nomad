# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

variable "num_nodes" {
  type = string
}

variable "server" {
  type = string
}

job "nodesim" {
  datacenters = ["dc1"]

  group "nodesim" {
    count = 20

    update {
      max_parallel = 3
    }

    task "nodesim" {
      driver = "docker"

      kill_signal = "SIGINT"

      config {
        image = "schmichael/nomad-nodesim:0.2"

        command = "/build/nomad-nodesim"
        args = [
          "-dir", "/local",
          "-num", var.num_nodes,
          "-server", var.server,
          "-id", "${NOMAD_SHORT_ALLOC_ID}",
        ]
      }

      resources {
        cpu    = 500
        memory = 500
      }
    }
  }
}
