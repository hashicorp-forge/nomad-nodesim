# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

variable "nomad_server_count" {
  default = 3
}

variable "nomad_server_instance_type" {
  default = "t2.medium"
}

variable "nomad_client_count" {
  default = 3
}

variable "nomad_client_instance_type" {
  default = "t2.medium"
}

variable "key_name" {
}
