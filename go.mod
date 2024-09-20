module github.com/linode/linode-blockstorage-csi-driver

go 1.22.5

require (
	github.com/container-storage-interface/spec v1.10.0
	github.com/go-logr/logr v1.4.1
	github.com/google/uuid v1.6.0
	github.com/ianschenck/envflag v0.0.0-20140720210342-9111d830d133
	github.com/linode/go-metadata v0.2.0
	github.com/linode/linodego v1.40.0
	github.com/martinjungblut/go-cryptsetup v0.0.0-20220520180014-fd0874fd07a6
	go.uber.org/automaxprocs v1.5.3
	go.uber.org/mock v0.4.0
	golang.org/x/net v0.29.0
	golang.org/x/sys v0.25.0
	google.golang.org/grpc v1.66.2
	google.golang.org/protobuf v1.34.2
	k8s.io/apimachinery v0.29.0
	k8s.io/klog/v2 v2.130.1
	k8s.io/mount-utils v0.30.3
	k8s.io/utils v0.0.0-20230726121419-3b25d923346b
)

require (
	github.com/go-resty/resty/v2 v2.13.1 // indirect
	github.com/moby/sys/mountinfo v0.6.2 // indirect
	golang.org/x/text v0.18.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240711142825-46eb208f015d // indirect
	gopkg.in/ini.v1 v1.66.6 // indirect
)
