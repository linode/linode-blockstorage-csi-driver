FROM alpine:3.7

RUN apk add --no-cache ca-certificates e2fsprogs findmnt

ADD linode-csi-plugin /bin/

ENTRYPOINT ["/bin/linode-csi-plugin"]
