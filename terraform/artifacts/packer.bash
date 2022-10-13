#!/usr/bin/bash

set -e

cd /ops

CONFIGDIR=/ops/shared/config

NOMADVERSION=1.4.1
NOMADDOWNLOAD=https://releases.hashicorp.com/nomad/${NOMADVERSION}/nomad_${NOMADVERSION}_linux_amd64.zip
NOMADCONFIGDIR=/etc/nomad.d
NOMADDIR=/opt/nomad

# Dependencies
sudo yum update -y
sudo yum install -y docker htop jq tree tmux

# Nomad
curl -fsSL $NOMADDOWNLOAD > nomad.zip

## Install
sudo unzip nomad.zip -d /usr/local/bin
sudo chmod 0755 /usr/local/bin/nomad
sudo chown root:root /usr/local/bin/nomad

## Configure
sudo mkdir -p $NOMADCONFIGDIR
sudo chmod 0755 $NOMADCONFIGDIR
sudo mkdir -p $NOMADDIR
sudo chmod 0755 $NOMADDIR
sudo cp /ops/base.hcl $NOMADCONFIGDIR/base.hcl
sudo chmod 0755 $NOMADCONFIGDIR/base.hcl

# CNI
curl -fsSL -o cni-plugins.tgz "https://github.com/containernetworking/plugins/releases/download/v1.1.1/cni-plugins-linux-amd64-v1.1.1.tgz"
sudo mkdir -p /opt/cni/bin
sudo tar -C /opt/cni/bin -xzf cni-plugins.tgz

# systemd
echo "Copying nomad unit file..."
sudo cp /ops/nomad.service /etc/systemd/system/nomad.service
echo "Reloading systemd..."
sudo systemctl daemon-reload
echo "Enabling nomad.service..."
sudo systemctl enable nomad
