variable "num_nodes" {
  type = string
}

variable "server" {
  type = string
}

job "nodesim" {
  datacenters = ["dc1"]

  group "nodesim" {
    task "nodesim" {
      driver = "docker"

      config {
        image = "schmichael/nomad-nodesim"

        command = "/build/nomad-nodesim"
        args = [
          "-dir", "local",
          "-num", var.num_nodes,
          "-server", var.server,
        ]
      }

      resources {
        cpu    = 2000
        memory = 1000
      }
    }
  }
}
