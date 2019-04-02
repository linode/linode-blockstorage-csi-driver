package framework

import (
	"github.com/appscode/go/crypto/rand"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	Image = "linode/linode-blockstorage-csi-driver:v0.1.0"
	ApiToken =""
)

type Framework struct {
	restConfig *rest.Config
	kubeClient kubernetes.Interface
	namespace    string
	name         string
	StorageClass string
}

func New(
	restConfig *rest.Config,
	kubeClient kubernetes.Interface,
	storageClass string,
	) *Framework {
	return &Framework{
		restConfig:restConfig,
		kubeClient:kubeClient,

		name:         "csidriver",
		namespace:    rand.WithUniqSuffix("csi"),
		StorageClass: storageClass,
	}
}

func (f *Framework) Invoke() *Invocation {
	return &Invocation{
		Framework: f,
		app:       rand.WithUniqSuffix("csi-driver-e2e"),
	}
}

type Invocation struct {
	*Framework
	app string
}
