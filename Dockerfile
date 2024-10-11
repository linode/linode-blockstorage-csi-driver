FROM golang:1.23.2-alpine AS builder
# from makefile
ARG REV

RUN mkdir -p /linode
WORKDIR /linode

COPY go.mod .
COPY go.sum .
COPY main.go .
COPY pkg ./pkg
COPY internal ./internal
RUN apk add cryptsetup cryptsetup-libs cryptsetup-dev gcc musl-dev pkgconfig

RUN go mod download

RUN CGO_ENABLED=1 go build -a -ldflags '-X main.vendorVersion='${REV}'' -o /bin/linode-blockstorage-csi-driver /linode

FROM alpine:3.20.3
LABEL maintainers="Linode"
LABEL description="Linode CSI Driver"

COPY --from=builder /bin/linode-blockstorage-csi-driver /linode

RUN apk add --no-cache e2fsprogs findmnt blkid cryptsetup
RUN apk add --no-cache xfsprogs=6.2.0-r2 --repository=http://dl-cdn.alpinelinux.org/alpine/v3.18/main

COPY --from=builder /bin/linode-blockstorage-csi-driver /linode
ENTRYPOINT ["/linode"]
