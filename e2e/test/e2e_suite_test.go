package test

import (
	"flag"
	"os"
	"testing"
	"time"

	"github.com/appscode/go/crypto/rand"
	"k8s.io/client-go/util/homedir"

	"github.com/linode/linodego"

	"path/filepath"

	"e2e_test/test/framework"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	useExisting = false
	reuse       = false
	clusterName string
	linodeDebug = false
	linodeURL   = "https://api.linode.com"
)

func init() {
	flag.StringVar(&framework.K8sVersion, "k8s-version", framework.K8sVersion, "Kubernetes version")
	flag.BoolVar(&reuse, "reuse", reuse, "Create a cluster and continue to use it")
	flag.BoolVar(&useExisting, "use-existing", useExisting, "Use an existing kubernetes cluster")
	flag.StringVar(&framework.KubeConfigFile, "kubeconfig", filepath.Join(homedir.HomeDir(), ".kube/config"), "To use existing cluster provide kubeconfig file")
	flag.DurationVar(&framework.Timeout, "timeout", 5*time.Minute, "Timeout for a test to complete successfully")
	flag.DurationVar(&framework.RetryInterval, "retry-interval", 5*time.Second, "Amount of time to wait between requests")
	flag.BoolVar(&linodeDebug, "linode-debug", linodeDebug, "When true, prints out HTTP requests and responses from the Linode API")
	flag.StringVar(&framework.ApiToken, "api-token", os.Getenv("LINODE_API_TOKEN"), "The authentication token to use when sending requests to the Linode API")
	flag.StringVar(&linodeURL, "linode-url", linodeURL, "The Linode API URL to send requests to")
}

var (
	root *framework.Framework
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(framework.Timeout)

	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "e2e Suite", []Reporter{junitReporter})
}

var getLinodeClient = func() linodego.Client {
	linodeClient := linodego.NewClient(nil)
	linodeClient.SetDebug(linodeDebug)
	linodeClient.SetBaseURL(linodeURL)
	linodeClient.SetAPIVersion("v4")
	linodeClient.SetToken(framework.ApiToken)
	linodeClient.SetUserAgent("csi-e2e")

	return linodeClient
}

var _ = BeforeSuite(func() {
	if reuse {
		clusterName = "csi-linode-for-reuse"
	} else {
		clusterName = rand.WithUniqSuffix("csi-linode")
	}

	dir, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	kubeConfigFile := filepath.Join(dir, clusterName+".conf")

	if reuse {
		if _, err := os.Stat(kubeConfigFile); !os.IsNotExist(err) {
			useExisting = true
			framework.KubeConfigFile = kubeConfigFile
		}
	}

	if !useExisting {
		Expect(framework.K8sVersion).NotTo(BeEmpty(), "Please specify a Kubernetes version")
		err := framework.CreateCluster(clusterName)
		Expect(err).NotTo(HaveOccurred())
		framework.KubeConfigFile = kubeConfigFile
	}

	By("Using kubeconfig from " + framework.KubeConfigFile)
	config, err := clientcmd.BuildConfigFromFlags("", framework.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred())

	// Clients
	kubeClient := kubernetes.NewForConfigOrDie(config)
	Expect(framework.ApiToken).NotTo(BeEmpty(), "API token is necessary")
	linodeClient := getLinodeClient()

	// Framework
	root = framework.New(config, kubeClient, linodeClient)

	By("Using Namespace " + root.Namespace())
	err = root.CreateNamespace()
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if root == nil {
		return
	}

	By("Deleting Namespace " + root.Namespace())
	err := root.DeleteNamespace(root.Namespace())
	Expect(err).NotTo(HaveOccurred())
	if !(useExisting || reuse) {
		By("Deleting cluster")
		err := framework.DeleteCluster(clusterName)
		Expect(err).NotTo(HaveOccurred())
	} else {
		By("Not deleting cluster")
	}
})
