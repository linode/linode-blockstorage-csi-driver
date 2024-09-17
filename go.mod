module github.com/linode/linode-blockstorage-csi-driver

go 1.22.5

require (
	github.com/container-storage-interface/spec v1.10.0
	github.com/go-logr/logr v1.4.1
	github.com/google/uuid v1.6.0
	github.com/ianschenck/envflag v0.0.0-20140720210342-9111d830d133
	github.com/linode/go-metadata v0.2.0
	github.com/linode/linodego v1.40.0
	go.uber.org/automaxprocs v1.5.3
	go.uber.org/mock v0.4.0
	golang.org/x/net v0.28.0
	golang.org/x/sys v0.25.0
	google.golang.org/grpc v1.65.0
	google.golang.org/protobuf v1.34.2
	k8s.io/apimachinery v0.19.2
	k8s.io/klog/v2 v2.130.1
	k8s.io/mount-utils v0.30.3
	k8s.io/utils v0.0.0-20230726121419-3b25d923346b
)

require (
	github.com/go-resty/resty/v2 v2.13.1 // indirect
	github.com/moby/sys/mountinfo v0.6.2 // indirect
	golang.org/x/text v0.17.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240711142825-46eb208f015d // indirect
	gopkg.in/ini.v1 v1.66.6 // indirect
)

replace (
	k8s.io/api => k8s.io/api v0.19.2
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.19.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.19.2
	k8s.io/apiserver => k8s.io/apiserver v0.19.2
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.19.2
	k8s.io/client-go => k8s.io/client-go v0.19.2
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.19.2
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.19.2
	k8s.io/code-generator => k8s.io/code-generator v0.19.2
	k8s.io/component-base => k8s.io/component-base v0.19.2
	k8s.io/cri-api => k8s.io/cri-api v0.19.2
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.19.2
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.19.2
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.19.2
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.19.2
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.19.2
	k8s.io/kubectl => k8s.io/kubectl v0.19.2
	k8s.io/kubelet => k8s.io/kubelet v0.19.2
	k8s.io/kubernetes => k8s.io/kubernetes v1.19.2
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.19.2
	k8s.io/metrics => k8s.io/metrics v0.19.2
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.19.2
)
