module github.com/linode/linode-blockstorage-csi-driver

go 1.21.5

require (
	github.com/container-storage-interface/spec v1.3.0
	github.com/linode/linodego v1.26.0
	golang.org/x/net v0.19.0
	golang.org/x/sys v0.15.0
	google.golang.org/grpc v1.60.1
	google.golang.org/protobuf v1.31.0
	k8s.io/apimachinery v0.29.0
	k8s.io/klog/v2 v2.110.1
	k8s.io/utils v0.0.0-20230726121419-3b25d923346b
)

require (
	github.com/go-logr/logr v1.3.0 // indirect
	github.com/go-resty/resty/v2 v2.10.0 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231212172506-995d672761c0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
)
