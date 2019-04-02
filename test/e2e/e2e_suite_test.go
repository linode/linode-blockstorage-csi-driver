package e2e

import (
	"flag"
	"github.com/appscode/go/crypto/rand"
	"testing"
	"time"

	"github.com/linode/linode-blockstorage-csi-driver/test/e2e/framework"
	"github.com/onsi/ginkgo/reporters"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"os"
	"path/filepath"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	StorageCluass = "linode-block-storage"
	useExisting = false
	kubecofigFile = filepath.Join(homedir.HomeDir(), ".kube/config")
	ClusterName = rand.WithUniqSuffix("csi-linode")
)

func init()  {
	flag.StringVar(&framework.Image, "image", framework.Image, "registry/repository:tag")
	flag.StringVar(&framework.ApiToken, "api-token", os.Getenv("LINODE_API_TOKEN"), "linode api token")

	flag.BoolVar(&useExisting, "use-existing", useExisting, "Use existing kubernetes cluster")
	flag.StringVar(&kubecofigFile, "kubeconfig", kubecofigFile, "To use existing cluster provide kubeconfig file" )
}

const (
	TIMEOUT = 20 * time.Minute
)

var (
	root *framework.Framework
)

func TestE2e(t *testing.T) {
	//logs.InitLogs()
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(TIMEOUT)

	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "e2e Suite", []Reporter{junitReporter})
}



var _ = BeforeSuite (func() {

	if !useExisting {
		err := framework.CreateCluster(ClusterName)
		Expect(err).NotTo(HaveOccurred())
		dir, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		kubecofigFile = filepath.Join(dir, ClusterName+".conf")
	}

	By("Using kubeconfig from " + kubecofigFile)
	config, err := clientcmd.BuildConfigFromFlags("", kubecofigFile)
	Expect(err).NotTo(HaveOccurred())

	// Clients
	kubeClient := kubernetes.NewForConfigOrDie(config)

	// Framework
	root = framework.New(config, kubeClient, StorageCluass)

	By("Using namespace " + root.Namespace())

	// Create namespace
	err = root.CreateNamespace()
	Expect(err).NotTo(HaveOccurred())

	err = root.ApplyManifest()
	Expect(err).NotTo(HaveOccurred())

})

var _ = AfterSuite(func() {
	if !useExisting {
		err := framework.DeleteCluster()
		Expect(err).NotTo(HaveOccurred())
	}
	err := root.DeleteManifest()
	Expect(err).NotTo(HaveOccurred())


})