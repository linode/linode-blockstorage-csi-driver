FROM golang:1.19-alpine as builder
# from makefile
ARG REV

RUN mkdir -p /linode
WORKDIR /linode

COPY go.mod .
COPY go.sum .

COPY app/linode/main.go .
COPY pkg ./pkg

ENV GO111MODULE=on
RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-X main.vendorVersion='${REV}' -extldflags "-static"' -o /bin/linode-blockstorage-csi-driver /linode

FROM alpine:3.16.2
LABEL maintainers="Linode"
LABEL description="Linode CSI Driver"

COPY --from=builder /bin/linode-blockstorage-csi-driver /linode

RUN apk add --no-cache e2fsprogs findmnt blkid cryptsetup

ENTRYPOINT ["/linode"]
