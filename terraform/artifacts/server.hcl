# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

server {
  enabled = true

  bootstrap_expect = 3

  server_join {
    retry_join = [ "provider=aws tag_key=AutoJoin tag_value=nomad addr_type=private_v4" ]
  }
}

limits {
  http_max_conns_per_client = 0
  rpc_max_conns_per_client  = 0
}
