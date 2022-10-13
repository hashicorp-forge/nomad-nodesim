client {
  enabled = true

  server_join {
    retry_join = [ "provider=aws tag_key=AutoJoin tag_value=nomad addr_type=private_v4" ]
  }

  options = {
    "driver.raw_exec.enable" = "1"
  }
}
