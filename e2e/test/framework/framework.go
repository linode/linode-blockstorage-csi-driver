package framework

import (
	"time"

	"github.com/appscode/go/crypto/rand"
	"github.com/linode/linodego"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	ApiToken       = ""
	KubeConfigFile = ""
	K8sVersion     = ""
	Timeout        time.Duration
	RetryInterval  time.Duration
)

type Framework struct {
	restConfig *rest.Config
	kubeClient kubernetes.Interface
	namespace  string
	name       string

	linodeClient linodego.Client
}

func New(
	restConfig *rest.Config,
	kubeClient kubernetes.Interface,
	linodeClient linodego.Client,
) *Framework {
	return &Framework{
		restConfig:   restConfig,
		kubeClient:   kubeClient,
		linodeClient: linodeClient,

		name:      "csidriver",
		namespace: rand.WithUniqSuffix("csi"),
	}
}

func (f *Framework) Invoke() *Invocation {
	return &Invocation{
		Framework:     f,
		Timeout:       Timeout,
		RetryInterval: RetryInterval,
		app:           rand.WithUniqSuffix("csi-driver-e2e"),
	}
}

type Invocation struct {
	*Framework
	Timeout       time.Duration
	RetryInterval time.Duration
	app           string
}
