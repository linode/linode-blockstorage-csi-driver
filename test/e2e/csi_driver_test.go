package e2e

import (
	"github.com/linode/linode-blockstorage-csi-driver/test/e2e/framework"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
)

var _ = Describe ("CSIDriver", func() {
	var (
		err error
		pod *core.Pod
		pvc *core.PersistentVolumeClaim
		f *framework.Invocation
	)
	BeforeEach(func() {
		f = root.Invoke()
	})

	Describe("Test", func() {
		Context("Simple", func() {
			Context("Block storage", func() {
				BeforeEach(func() {
					By("Creating persistent volume claim")
					pvc = f.GetPersistentVolumeClaim()
					err = f.CreatePersistentVolumeClaim(pvc)
					Expect(err).NotTo(HaveOccurred())

					By("Creating pod with pvc")
					pod = f.GetPodObject(pvc.Name)
					err = f.CreatePod(pod)
					Expect(err).NotTo(HaveOccurred())
				})

				AfterEach(func() {
					err = f.DeletePod(pod.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

					err = f.DeletePersistentVolumeClaim(pvc.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())
				})

				It("should write and read", func() {
					filename := "/data/heredoc"
					err = f.WriteFileIntoPod(filename, pod)
					Expect(err).NotTo(HaveOccurred())

					err = f.CheckFileIntoPod(filename, pod)
					Expect(err).NotTo(HaveOccurred())
				})


			})
		})
	})

})