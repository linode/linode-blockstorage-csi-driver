FROM golang:1.25.0-alpine AS builder
# from makefile
ARG REV

RUN mkdir -p /linode
WORKDIR /linode

RUN apk add cryptsetup cryptsetup-libs cryptsetup-dev gcc musl-dev pkgconfig

COPY go.mod go.sum ./
RUN go mod download

COPY main.go .
COPY pkg ./pkg
COPY internal ./internal

RUN CGO_ENABLED=1 go build -a -ldflags "-w -s -X main.vendorVersion=${REV}" -o /bin/linode-blockstorage-csi-driver /linode

FROM alpine:3.20.3
LABEL maintainers="Linode"
LABEL description="Linode CSI Driver"

RUN apk add --no-cache e2fsprogs e2fsprogs-extra findmnt blkid cryptsetup
RUN apk add --no-cache xfsprogs=6.2.0-r2 xfsprogs-extra=6.2.0-r2 --repository=http://dl-cdn.alpinelinux.org/alpine/v3.18/main

COPY --from=builder /bin/linode-blockstorage-csi-driver /linode

ENTRYPOINT ["/linode"]
