package framework

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	core "k8s.io/api/core/v1"

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

	VolumeTypeRequiredError = fmt.Errorf("volumeType is required and must be one of [%s, %s]",
		core.PersistentVolumeFilesystem, core.PersistentVolumeBlock)
)

type Framework struct {
	ctx        context.Context
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
		ctx:          context.Background(),
		restConfig:   restConfig,
		kubeClient:   kubeClient,
		linodeClient: linodeClient,

		name:      "csidriver",
		namespace: fmt.Sprintf("csi-%x", rand.Int31()),
	}
}

func (f *Framework) Invoke() *Invocation {
	return &Invocation{
		Framework:     f,
		Timeout:       Timeout,
		RetryInterval: RetryInterval,
		app:           fmt.Sprintf("csi-driver-e2e-%x", rand.Int31()),
	}
}

type Invocation struct {
	*Framework
	Timeout       time.Duration
	RetryInterval time.Duration
	app           string
}
