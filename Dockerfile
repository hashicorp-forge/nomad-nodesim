# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# devbuild compiles the binary
# -----------------------------------
FROM golang:1.24 AS devbuild

WORKDIR /build
COPY . ./
ENV CGO_ENABLED=1
RUN go build -tags hashicorpmetrics -o nomad-nodesim .

# dev runs the binary from devbuild
# -----------------------------------
FROM debian:stable AS dev

RUN apt update
RUN apt install -y \
    iptables \
    iproute2

COPY --from=devbuild /build/nomad-nodesim /bin/
