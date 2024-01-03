# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

data "amazon-ami" "base-image" {
  filters = {
    architecture                       = "x86_64"
    "block-device-mapping.volume-type" = "gp2"
    name                               = "amzn2-ami-kernel-5.10-hvm-2.0.20220912.1-x86_64-gp2"
    root-device-type                   = "ebs"
    virtualization-type                = "hvm"
  }
  most_recent = true
  owners      = ["amazon"]
  region      = "us-west-2"
}

locals { timestamp = regex_replace(timestamp(), "[- TZ:]", "") }

source "amazon-ebs" "base-volume" {
  ami_name      = "nomad-base-${local.timestamp}"
  instance_type = "t2.medium"
  region        = "us-west-2"
  source_ami    = "${data.amazon-ami.base-image.id}"
  ssh_username  = "ec2-user"
}

build {
  sources = ["source.amazon-ebs.base-volume"]

  provisioner "shell" {
    inline = ["sudo mkdir /ops", "sudo chmod 777 /ops"]
  }

  provisioner "file" {
    source      = "artifacts/"
    destination = "/ops"
  }

  provisioner "shell" {
    script = "artifacts/packer.bash"
  }
}
