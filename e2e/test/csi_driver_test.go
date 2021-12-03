package test

import (
	"e2e_test/test/framework"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var _ = Describe("CSIDriver", func() {
	var (
		err          error
		pod          *core.Pod
		pvc          *core.PersistentVolumeClaim
		f            *framework.Invocation
		size         string
		file         = "/data/heredoc"
		storageClass = "linode-block-storage"
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

	var expandVolume = func(size string) {
		By("Expanding Size of the Persistent Volume")
		currentPVC, err := f.GetPersistentVolumeClaim(pvc.ObjectMeta)
		Expect(err).NotTo(HaveOccurred())

		currentPVC.Spec.Resources.Requests = core.ResourceList{
			core.ResourceName(core.ResourceStorage): resource.MustParse(size),
		}
		err = f.UpdatePersistentVolumeClaim(currentPVC)
		Expect(err).NotTo(HaveOccurred())

		By("Checking if Volume Expansion Occurred")
		Eventually(func() string {
			s, _ := f.GetVolumeSize(currentPVC)
			return strconv.Itoa(s) + "Gi"
		}, f.Timeout, f.RetryInterval).Should(Equal(size))
	}

	Describe("Test", func() {
		Context("Simple", func() {
			Context("Block Storage", func() {
				JustBeforeEach(func() {
					By("Creating Persistent Volume Claim")
					pvc = f.GetPersistentVolumeClaimObject(size, storageClass)
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

					By("Getting the Volume information")
					currentPVC, err := f.GetPersistentVolumeClaim(pvc.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())
					volumeID, err := f.GetVolumeID(currentPVC)
					Expect(err).NotTo(HaveOccurred())

					By("Waiting for the Volume to be Detached")
					Eventually(func() bool {
						isAttached, err := f.IsVolumeDetached(volumeID)
						if err != nil {
							return false
						}
						return isAttached
					}, f.Timeout, f.RetryInterval).Should(BeTrue())

					By("Deleting the PVC")
					err = f.DeletePersistentVolumeClaim(pvc.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

					By("Waiting for the Volume to be Deleted")
					Eventually(func() bool {
						isDeleted, err := f.IsVolumeDeleted(volumeID)
						if err != nil {
							return false
						}
						return isDeleted
					}, f.Timeout, f.RetryInterval).Should(BeTrue())
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
			})
		})
	})

	Describe("Test", func() {
		Context("Block Storage", func() {
			Context("Volume Expansion", func() {
				JustBeforeEach(func() {
					By("Creating Persistent Volume Claim")
					pvc = f.GetPersistentVolumeClaimObject(size, storageClass)
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

					By("Getting the Volume information")
					currentPVC, err := f.GetPersistentVolumeClaim(pvc.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())
					volumeID, err := f.GetVolumeID(currentPVC)
					Expect(err).NotTo(HaveOccurred())

					By("Waiting for the Volume to be Detached")
					Eventually(func() bool {
						isDetached, err := f.IsVolumeDetached(volumeID)
						if err != nil {
							return false
						}
						return isDetached
					}, f.Timeout, f.RetryInterval).Should(BeTrue())

					By("Deleting the PVC")
					err = f.DeletePersistentVolumeClaim(pvc.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

					By("Waiting for the Volume to be Deleted")
					Eventually(func() bool {
						isDeleted, err := f.IsVolumeDeleted(volumeID)
						if err != nil {
							return false
						}
						return isDeleted
					}, f.Timeout, f.RetryInterval).Should(BeTrue())
				})

				Context("Expanding Storage from 10Gi to 15Gi", func() {
					BeforeEach(func() {
						size = "10Gi"
					})
					It("should write and read", func() {
						writeFile(file)
						expandVolume("15Gi")
						readFile(file)
					})
				})
			})
		})
	})
})
