FROM alpine
LABEL maintainers="Linode"
LABEL description="Linode CSI Driver"

RUN apk add --no-cache ca-certificates e2fsprogs findmnt

COPY ./_output/linode /linode

ENTRYPOINT ["/linode"]
