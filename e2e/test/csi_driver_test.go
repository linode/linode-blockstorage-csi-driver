package test

import (
	"e2e_test/framework"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
)

var _ = Describe("CSIDriver", func() {
	var (
		err      error
		pod      *core.Pod
		podName1 = "test-pod-1"
		podName2 = "test-pod-2"
		pvc      *core.PersistentVolumeClaim
		f        *framework.Invocation
		size     string
		file     = "/data/heredoc"
		waitTime = 1 * time.Minute
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

	var waitForOperation = func() {
		time.Sleep(waitTime)
	}

	var deleteAndCreatePod = func(name string) {
		By("Deleting the First pod")
		err = f.DeletePod(pod.Name)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for the Volume to be Detached")
		waitForOperation()

		By("Creating Second Pod with the Same PVC")
		pod = f.GetPodObject(name, pvc.Name)
		err = f.CreatePod(pod)
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
					pod = f.GetPodObject(podName1, pvc.Name)
					err = f.CreatePod(pod)
					Expect(err).NotTo(HaveOccurred())
				})

				AfterEach(func() {
					By("Deleting the Pod with PVC")
					err = f.DeletePod(pod.Name)
					Expect(err).NotTo(HaveOccurred())

					By("Waiting for the Volume to be Detached")
					waitForOperation()

					By("Deleting the PVC")
					err = f.DeletePersistentVolumeClaim(pvc.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

					By("Waiting for the Volume to be Deleted")
					waitForOperation()
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

		Context("Pre-Provisioned", func() {
			Context("Linode Block Storage", func() {
				JustBeforeEach(func() {
					By("Creating Persistent Volume Claim")
					pvc = f.GetPersistentVolumeClaim(size)
					err = f.CreatePersistentVolumeClaim(pvc)
					Expect(err).NotTo(HaveOccurred())

					By("Creating Pod with PVC")
					pod = f.GetPodObject(podName1, pvc.Name)
					err = f.CreatePod(pod)
					Expect(err).NotTo(HaveOccurred())
				})

				AfterEach(func() {
					By("Deleting the Pod with PVC")
					err = f.DeletePod(pod.Name)
					Expect(err).NotTo(HaveOccurred())

					By("Waiting for the Volume to be Detached")
					waitForOperation()

					By("Deleting the PVC")
					err = f.DeletePersistentVolumeClaim(pvc.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

					By("Waiting for the Volume to be Deleted")
					waitForOperation()
				})

				Context("10Gi Storage", func() {
					BeforeEach(func() {
						size = "10Gi"
					})
					It("should write and read", func() {
						writeFile(file)
						deleteAndCreatePod(podName2)
						readFile(file)
					})
				})

				Context("15Gi Storage", func() {
					BeforeEach(func() {
						size = "15Gi"
					})
					It("should write and read", func() {
						writeFile(file)
						deleteAndCreatePod(podName2)
						readFile(file)
					})
				})

				Context("20Gi Storage", func() {
					BeforeEach(func() {
						size = "20Gi"
					})
					It("should write and read", func() {
						writeFile(file)
						deleteAndCreatePod(podName2)
						readFile(file)
					})
				})
			})
		})
	})

})
