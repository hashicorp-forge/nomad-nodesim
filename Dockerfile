FROM golang:1.19
RUN apt update
RUN apt install -y iproute2
WORKDIR /build
COPY . ./
ENV CGO_ENABLED=1
RUN go build
