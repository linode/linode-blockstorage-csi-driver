package test

import (
	"e2e_test/framework"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	"time"
)

var _ = Describe("CSIDriver", func() {
	var (
		err error
		pod *core.Pod
		pvc *core.PersistentVolumeClaim
		f   *framework.Invocation
		size string
		file = "/data/heredoc"
	)
	BeforeEach(func() {
		f = root.Invoke()
	})

	var writeFile = func(filename string) {
		By("Writing a File into the Pod")
		err = f.WriteFileIntoPod(filename, pod)
		Expect(err).NotTo(HaveOccurred())
	}

	var readFile = func(filename string) {
		By("Checking the Created File into the Pod")
		err = f.CheckFileIntoPod(filename, pod)
		Expect(err).NotTo(HaveOccurred())
	}

	Describe("Test", func() {
		Context("Simple", func() {
			Context("Block Storage", func() {
				JustBeforeEach(func() {
					By("Creating Persistent Volume Claim")
					pvc = f.GetPersistentVolumeClaim(size)
					err = f.CreatePersistentVolumeClaim(pvc)
					Expect(err).NotTo(HaveOccurred())

					By("Creating Pod with PVC")
					pod = f.GetPodObject(pvc.Name)
					err = f.CreatePod(pod)
					Expect(err).NotTo(HaveOccurred())
				})

				AfterEach(func() {
					By("Deleting the Pod with PVC")
					err = f.DeletePod(pod.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

					By("Waiting for the Volume to be Detached")
					time.Sleep(2*time.Minute)

					By("Deleting the PVC")
					err = f.DeletePersistentVolumeClaim(pvc.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

					By("Waiting for the Volume to be Deleted")
					time.Sleep(1*time.Minute)
				})

				Context("1Gi Storage", func() {
					BeforeEach(func() {
						size = "1Gi"
					})
					It("should write and read", func() {
						writeFile(file)
						readFile(file)
					})
				})

				Context("10Gi Storage", func() {
					BeforeEach(func() {
						size = "10Gi"
					})
					It("should write and read", func() {
						writeFile(file)
						readFile(file)
					})
				})

				Context("20Gi Storage", func() {
					BeforeEach(func() {
						size = "20Gi"
					})
					It("should write and read", func() {
						writeFile(file)
						readFile(file)
					})
				})
			})
		})
	})

})
