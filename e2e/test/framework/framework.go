package framework

import (
	"github.com/appscode/go/crypto/rand"
	"github.com/linode/linodego"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	Image          = "linode/linode-blockstorage-csi-driver:latest"
	ApiToken       = ""
	KubeConfigFile = ""
)

type Framework struct {
	restConfig   *rest.Config
	kubeClient   kubernetes.Interface
	namespace    string
	name         string
	StorageClass string

	linodeClient linodego.Client
}

func New(
	restConfig *rest.Config,
	kubeClient kubernetes.Interface,
	linodeClient linodego.Client,
	storageClass string,
) *Framework {
	return &Framework{
		restConfig:   restConfig,
		kubeClient:   kubeClient,
		linodeClient: linodeClient,

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
