server {
  enabled = true

  bootstrap_expect = 3

  server_join {
    retry_join = [ "provider=aws tag_key=AutoJoin tag_value=nomad addr_type=private_v4" ]
  }
}
