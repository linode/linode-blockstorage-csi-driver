FROM golang:1.26-alpine

RUN mkdir -p /linode
WORKDIR /linode

RUN apk add bash mise cryptsetup cryptsetup-libs cryptsetup-dev gcc musl-dev pkgconfig

COPY go.mod go.sum ./
RUN go mod download

COPY mise.toml mise.toml

COPY main.go .
COPY pkg ./pkg
COPY internal ./internal
