# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

resource "random_pet" "nodesim" {}

# Create an IAM role for the auto-join
resource "aws_iam_role" "nomad-nodesim" {
  name = "nomad-nodesim-${random_pet.nodesim.id}"

  assume_role_policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Action": "sts:AssumeRole",
      "Principal": {
        "Service": "ec2.amazonaws.com"
      },
      "Effect": "Allow",
      "Sid": ""
    }
  ]
}
EOF
}

resource "aws_iam_policy" "nomad-nodesim" {
  name        = "nomad-nodesim-${random_pet.nodesim.id}"
  description = "Allows nodes to describe instances for joining."

  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeInstances"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action":"s3:GetObject",
      "Resource": "arn:aws:s3:::nomad-nodesim/*"
    }
  ]
}
EOF
}

# Attach the policy
resource "aws_iam_policy_attachment" "nomad-nodesim" {
  name       = "nomad-nodesim-${random_pet.nodesim.id}"
  roles      = ["${aws_iam_role.nomad-nodesim.name}"]
  policy_arn = aws_iam_policy.nomad-nodesim.arn
}

resource "aws_iam_policy_attachment" "nomad-nodesim-ssm" {
  name       = "nomad-nodesim-${random_pet.nodesim.id}-ssm"
  roles      = ["${aws_iam_role.nomad-nodesim.name}"]
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

# Create the instance profile
resource "aws_iam_instance_profile" "nomad_nodesim" {
  name = "nomad-nodesim-${random_pet.nodesim.id}"
  role = aws_iam_role.nomad-nodesim.name
}

data "aws_ami" "nomad_base_ami" {
  most_recent = true
  owners      = ["self"]

  filter {
    name   = "name"
    values = ["nomad-base-*"]
  }
}

data "aws_caller_identity" "current" {
}

## Compute

data "local_file" "client_hcl" {
  filename = "artifacts/client.hcl"
}

data "local_file" "server_hcl" {
  filename = "artifacts/server.hcl"
}

resource "aws_instance" "server" {
  ami                    = data.aws_ami.nomad_base_ami.image_id
  instance_type          = var.nomad_server_instance_type
  key_name               = var.key_name
  vpc_security_group_ids = [aws_security_group.primary.id]
  count                  = var.nomad_server_count
  iam_instance_profile   = aws_iam_instance_profile.nomad_nodesim.name

  # Instance tags
  tags = {
    Name     = "${random_pet.nodesim.id}-server-${count.index}"
    AutoJoin = "nomad"
    User     = data.aws_caller_identity.current.arn
  }

  user_data = templatefile("artifacts/userdata.bash.tftpl", {
    client = false
    server = true
    client_hcl = data.local_file.client_hcl.content
    server_hcl = data.local_file.server_hcl.content
  })
}

resource "aws_instance" "client" {
  ami                    = data.aws_ami.nomad_base_ami.image_id
  instance_type          = var.nomad_client_instance_type
  key_name               = var.key_name
  vpc_security_group_ids = [aws_security_group.primary.id]
  count                  = var.nomad_client_count
  iam_instance_profile   = aws_iam_instance_profile.nomad_nodesim.name

  user_data = templatefile("artifacts/userdata.bash.tftpl", {
    client = true
    server = false
    client_hcl = data.local_file.client_hcl.content
    server_hcl = data.local_file.server_hcl.content
  })

  # Instance tags
  tags = {
    Name     = "${random_pet.nodesim.id}-client-${count.index}"
    User     = data.aws_caller_identity.current.arn
  }
}
