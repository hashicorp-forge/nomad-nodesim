# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

FROM golang:1.19
RUN apt update
RUN apt install -y iproute2
WORKDIR /build
COPY . ./
ENV CGO_ENABLED=1
RUN go build
