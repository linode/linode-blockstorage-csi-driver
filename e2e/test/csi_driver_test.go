package test

import (
	"e2e_test/test/framework"
	"fmt"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Linode CSI Driver", func() {
	Describe("A StatefulSet with a PVC", func() {
		Context("Using VolumeClaimTemplates and a non-root container", func() {
			var (
				f            *framework.Invocation
				storageClass = "linode-block-storage"
				sts          *appsv1.StatefulSet
				pod          *core.Pod
				pvc          *core.PersistentVolumeClaim
				file         = "/data/file.txt"
			)

			BeforeEach(func() {
				f = root.Invoke()
				By("Getting the StatefulSet manifest w/ non-root container")
				sts = framework.GetStatefulSetObject("redis-test", f.Namespace(), storageClass)

				By("Creating the StatefulSet in the cluster")
				Eventually(func() error {
					return f.CreateStatefulSet(sts)
				}, f.Timeout, f.RetryInterval).Should(Succeed())

				By("Waiting until the StatefulSet Pod is healthy")
				Eventually(func() error {
					var err error
					pod, err = f.GetPod("redis-test-0", f.Namespace())
					if err != nil {
						return err
					}
					return f.WaitForReady(pod.ObjectMeta)
				}, f.Timeout, f.RetryInterval).Should(Succeed())

				By("Checking that there is a PVC created for the StatefulSet")
				Eventually(func() error {
					var err error
					pvc, err = f.GetPersistentVolumeClaim(metav1.ObjectMeta{Name: "data-redis-test-0", Namespace: f.Namespace()})
					if err != nil {
						return err
					}
					return nil
				}, f.Timeout, f.RetryInterval).Should(Succeed())
			})

			AfterEach(func() {
				By("Deleting the StatefulSet")
				Eventually(func() error {
					return f.DeleteStatefulSet(sts.ObjectMeta)
				}, f.Timeout, f.RetryInterval).Should(Succeed())

				By("Getting the Volume information")
				var (
					currentPVC *core.PersistentVolumeClaim
					volumeID   int
				)
				Eventually(func() error {
					var err error
					currentPVC, err = f.GetPersistentVolumeClaim(pvc.ObjectMeta)
					if err != nil {
						return err
					}
					volumeID, err = f.GetVolumeID(currentPVC)
					if err != nil {
						return err
					}
					return nil
				}, f.Timeout, f.RetryInterval).Should(Succeed())

				By("Waiting for the Volume to be Detached")
				Eventually(func() bool {
					isAttached, err := f.IsVolumeDetached(volumeID)
					if err != nil {
						return false
					}
					return isAttached
				}, f.Timeout, f.RetryInterval).Should(BeTrue())

				By("Deleting the PVC")
				Eventually(func() error {
					return f.DeletePersistentVolumeClaim(pvc.ObjectMeta)
				}, f.Timeout, f.RetryInterval).Should(Succeed())

				By("Waiting for the Volume to be Deleted")
				Eventually(func() bool {
					isDeleted, err := f.IsVolumeDeleted(volumeID)
					if err != nil {
						return false
					}
					return isDeleted
				}, f.Timeout, f.RetryInterval).Should(BeTrue())
			})

			It("Ensures no data is lost between Pod deletions", func() {
				var err error
				By("Saving a file in the mounted directory within the container")
				err = f.WriteFileIntoPod(file, pod)
				Expect(err).NotTo(HaveOccurred())

				By("Deleting the StatefulSet Pod")
				Eventually(func() error {
					return f.DeletePod(pod.ObjectMeta)
				}, f.Timeout, f.RetryInterval).Should(Succeed())

				By("Waiting until the StatefulSet Pod is recreated")
				Eventually(func() error {
					name := "redis-test-0"
					p, err := f.GetPod(name, f.Namespace())
					if err != nil {
						return err
					}
					if p.ObjectMeta.UID == pod.ObjectMeta.UID {
						return fmt.Errorf("pod %s/%s not deleted", f.Namespace(), name)
					}
					pod = p
					return nil
				}, f.Timeout, f.RetryInterval).Should(Succeed())
				Eventually(func() error {
					return f.WaitForReady(pod.ObjectMeta)
				}, f.Timeout, f.RetryInterval).Should(Succeed())

				By("Checking that the file is still present inside the container")
				err = f.CheckIfFileIsInPod(file, pod)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("A Pod with a PVC", func() {
		var (
			f            *framework.Invocation
			pod          *core.Pod
			pvc          *core.PersistentVolumeClaim
			size         string
			file         = "/data/heredoc"
			storageClass = "linode-block-storage"
		)

		var writeFile = func(filename string) {
			By("Writing a file into the Pod")
			err := f.WriteFileIntoPod(filename, pod)
			Expect(err).NotTo(HaveOccurred())
		}

		var readFile = func(filename string) {
			By("Checking if the created file is in the Pod")
			err := f.CheckIfFileIsInPod(filename, pod)
			Expect(err).NotTo(HaveOccurred())
		}

		var expandVolume = func(size string) {
			By("Expanding size of the Persistent Volume")
			currentPVC, err := f.GetPersistentVolumeClaim(pvc.ObjectMeta)
			Expect(err).NotTo(HaveOccurred())

			currentPVC.Spec.Resources.Requests = core.ResourceList{
				core.ResourceName(core.ResourceStorage): resource.MustParse(size),
			}
			err = f.UpdatePersistentVolumeClaim(currentPVC)
			Expect(err).NotTo(HaveOccurred())

			By("Checking if Volume expansion occurred")
			Eventually(func() string {
				s, _ := f.GetVolumeSize(currentPVC)
				return strconv.Itoa(s) + "Gi"
			}, f.Timeout, f.RetryInterval).Should(Equal(size))
		}

		Context("Using a Pod with a PVC mounted", func() {
			JustBeforeEach(func() {
				f = root.Invoke()
				By("Creating the Persistent Volume Claim")
				pvc = framework.GetPersistentVolumeClaimObject("test-pvc", f.Namespace(), size, storageClass)
				Eventually(func() error {
					return f.CreatePersistentVolumeClaim(pvc)
				}, f.Timeout, f.RetryInterval).Should(Succeed())

				By("Creating Pod with PVC")
				pod = framework.GetPodObject("busybox-test", f.Namespace(), pvc.Name)
				Eventually(func() error {
					return f.CreatePod(pod)
				}, f.Timeout, f.RetryInterval).Should(Succeed())
			})

			AfterEach(func() {
				By("Deleting the Pod with PVC")
				Eventually(func() error {
					return f.DeletePod(pod.ObjectMeta)
				}, f.Timeout, f.RetryInterval).Should(Succeed())

				By("Getting the Volume information")
				var (
					currentPVC *core.PersistentVolumeClaim
					volumeID   int
				)
				Eventually(func() error {
					var err error
					currentPVC, err = f.GetPersistentVolumeClaim(pvc.ObjectMeta)
					if err != nil {
						return err
					}
					volumeID, err = f.GetVolumeID(currentPVC)
					if err != nil {
						return err
					}
					return nil
				}, f.Timeout, f.RetryInterval).Should(Succeed())

				By("Waiting for the Volume to be Detached")
				Eventually(func() bool {
					isAttached, err := f.IsVolumeDetached(volumeID)
					if err != nil {
						return false
					}
					return isAttached
				}, f.Timeout, f.RetryInterval).Should(BeTrue())

				By("Deleting the PVC")
				Eventually(func() error {
					return f.DeletePersistentVolumeClaim(pvc.ObjectMeta)
				}, f.Timeout, f.RetryInterval).Should(Succeed())

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
