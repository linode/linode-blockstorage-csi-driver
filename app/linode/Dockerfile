FROM golang:1.20-alpine as builder
# from makefile
ARG REV

RUN mkdir -p /linode
WORKDIR /linode

COPY go.mod .
COPY go.sum .

COPY app/linode/main.go .
COPY pkg ./pkg

RUN go mod download

RUN go build -a -ldflags '-X main.vendorVersion='${REV}' -extldflags "-static"' -o /bin/linode-blockstorage-csi-driver /linode

FROM alpine:3.18.4
LABEL maintainers="Linode"
LABEL description="Linode CSI Driver"

COPY --from=builder /bin/linode-blockstorage-csi-driver /linode

RUN apk add --no-cache e2fsprogs findmnt blkid cryptsetup

ENTRYPOINT ["/linode"]
