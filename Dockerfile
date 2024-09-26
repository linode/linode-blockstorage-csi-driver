FROM golang:1.23.1-alpine AS builder
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

FROM alpine:3.18.4
LABEL maintainers="Linode"
LABEL description="Linode CSI Driver"

COPY --from=builder /bin/linode-blockstorage-csi-driver /linode

RUN apk add --no-cache e2fsprogs findmnt blkid cryptsetup xfsprogs
COPY --from=builder /bin/linode-blockstorage-csi-driver /linode
ENTRYPOINT ["/linode"]
