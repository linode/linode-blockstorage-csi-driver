package test

import (
	"fmt"
	"math/rand"
	"strconv"

	"e2e_test/test/framework"

	. "github.com/onsi/ginkgo/v2"
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
				sts          *appsv1.StatefulSet
				pod          *core.Pod
				pvc          *core.PersistentVolumeClaim
				file         = "/data/file.txt"
				storageClass = "linode-block-storage"
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
					return f.IsPodReady(pod.ObjectMeta)
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

				By("Deleting the StatefulSet")
				Eventually(func() error {
					return f.DeleteStatefulSet(sts.ObjectMeta)
				}, f.Timeout, f.RetryInterval).Should(Succeed())

				By("Waiting for the Volume to be Detached")
				Eventually(func() error {
					isDetached, err := f.IsVolumeDetached(volumeID)
					if err != nil {
						return err
					}
					if isDetached {
						return nil
					}
					return fmt.Errorf("volume %d is still attached", volumeID)
				}, f.Timeout, f.RetryInterval).Should(Succeed())

				By("Deleting the PVC")
				Eventually(func() error {
					return f.DeletePersistentVolumeClaim(pvc.ObjectMeta)
				}, f.Timeout, f.RetryInterval).Should(Succeed())

				By("Waiting for the Volume to be Deleted")
				Eventually(func() error {
					isDeleted, err := f.IsVolumeDeleted(volumeID)
					if err != nil {
						return err
					}
					if isDeleted {
						return nil
					}
					return fmt.Errorf("volume %d is still present", volumeID)
				}, f.Timeout, f.RetryInterval).Should(Succeed())
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
					return f.IsPodReady(pod.ObjectMeta)
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
			volumeType   core.PersistentVolumeMode
			writeFile    = func(filename string) {
				By("Writing a file into the Pod")
				err := f.WriteFileIntoPod(filename, pod)
				Expect(err).NotTo(HaveOccurred())
			}
			readFile = func(filename string) {
				By("Checking if the created file is in the Pod")
				err := f.CheckIfFileIsInPod(filename, pod)
				Expect(err).NotTo(HaveOccurred())
			}
			expandVolume = func(size string) {
				By("Expanding size of the Persistent Volume")
				currentPVC, err := f.GetPersistentVolumeClaim(pvc.ObjectMeta)
				Expect(err).NotTo(HaveOccurred())

				currentPVC.Spec.Resources.Requests = core.ResourceList{
					core.ResourceStorage: resource.MustParse(size),
				}
				err = f.UpdatePersistentVolumeClaim(currentPVC)
				Expect(err).NotTo(HaveOccurred())

				By("Checking if Volume expansion occurred")
				Eventually(func() (string, error) {
					s, err := f.GetVolumeSize(currentPVC)
					if err != nil {
						return "", err
					}
					return strconv.Itoa(s) + "Gi", nil
				}, f.Timeout, f.RetryInterval).Should(Equal(size))
			}
		)

		Context("Using a Pod with a PVC mounted", func() {
			JustBeforeEach(func() {
				f = root.Invoke()
				r := strconv.Itoa(rand.Intn(1024))
				var err error

				By("Creating the Persistent Volume Claim")
				pvc, err = f.GetPersistentVolumeClaimObject("test-pvc-"+r, f.Namespace(), size, storageClass, volumeType)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() error {
					return f.CreatePersistentVolumeClaim(pvc)
				}, f.Timeout, f.RetryInterval).Should(Succeed())

				By("Creating Pod with PVC")
				pod, err = f.GetPodObject("test-pod"+r, f.Namespace(), pvc.Name, volumeType)
				Expect(err).NotTo(HaveOccurred())

				Expect(f.CreatePod(pod)).To(Succeed())
				Eventually(func() error {
					return f.IsPodReady(pod.ObjectMeta)
				}, f.Timeout, f.RetryInterval).Should(Succeed())
			})

			AfterEach(func() {
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

				By("Deleting the Pod with PVC")
				Eventually(func() error {
					return f.DeletePod(pod.ObjectMeta)
				}, f.Timeout, f.RetryInterval).Should(Succeed())

				By("Waiting for the Volume to be Detached")
				Eventually(func() error {
					isDetached, err := f.IsVolumeDetached(volumeID)
					if err != nil {
						return err
					}
					if isDetached {
						return nil
					}
					return fmt.Errorf("volume %d is still attached", volumeID)
				}, f.Timeout, f.RetryInterval).Should(Succeed())

				By("Deleting the PVC")
				Eventually(func() error {
					return f.DeletePersistentVolumeClaim(pvc.ObjectMeta)
				}, f.Timeout, f.RetryInterval).Should(Succeed())

				By("Waiting for the Volume to be Deleted")
				Eventually(func() error {
					isDeleted, err := f.IsVolumeDeleted(volumeID)
					if err != nil {
						return err
					}
					if isDeleted {
						return nil
					}
					return fmt.Errorf("volume %d is still present", volumeID)
				}, f.Timeout, f.RetryInterval).Should(Succeed())
			})

			// filesystem
			Context("1Gi Filesystem Storage", func() {
				BeforeEach(func() {
					volumeType = core.PersistentVolumeFilesystem
					size = "1Gi"
				})
				It("should write and read", func() {
					writeFile(file)
					readFile(file)
				})
			})

			Context("Expanding Filesystem Storage from 10Gi to 15Gi", func() {
				BeforeEach(func() {
					volumeType = core.PersistentVolumeFilesystem
					size = "10Gi"
				})
				It("should write and read", func() {
					writeFile(file)
					expandVolume("15Gi")
					readFile(file)
				})
			})

			// raw block
			Context("1Gi Raw Block Storage", func() {
				BeforeEach(func() {
					volumeType = core.PersistentVolumeBlock
					size = "1Gi"
				})

				It("should check that raw block storage works", func() {
					By("Creating a ext4 Filesystem on the Pod")
					Expect(f.MkfsInPod(pod)).NotTo(HaveOccurred())
				})
			})

			Context("Expanding Raw Block Storage from 10Gi to 15Gi", func() {
				BeforeEach(func() {
					volumeType = core.PersistentVolumeBlock
					size = "10Gi"
				})
				It("should validate expansion", func() {
					expandVolume("15Gi")
				})
			})

			// LUKS
			Context("LUKS Encrypted Volume", func() {
				var (
					originalStorageClass string
					luksSecretName       string
				)
				BeforeEach(func() {
					volumeType = core.PersistentVolumeFilesystem
					size = "10Gi"

					// Create Secret
					luksSecretName = "luks"
					Eventually(func() error {
						return f.CreateLuksSecret(luksSecretName)
					}, f.Timeout, f.RetryInterval).Should(Succeed())

					// Create Storage Class
					params := f.GetLuksParameters(luksSecretName, "kube-system")
					sc := f.GetStorageClass("luks", params)
					Eventually(func() error {
						return f.CreateStorageClass(sc)
					}, f.Timeout, f.RetryInterval).Should(Succeed())

					originalStorageClass = storageClass
					storageClass = sc.Name
				})
				It("should validate LUKS volumes", func() {
					writeFile(file)
					readFile(file)
				})
				// AfterEach is required here because the
				// storage class and secret names are static.
				// This means that reuse of clusters causes
				// problems as Creates will fail due to the
				// objects already existing.
				AfterEach(func() {
					// Delete StorageClass
					Eventually(func() error {
						return f.DeleteStorageClass(storageClass)
					}).Should(Succeed())

					// Delete Secret
					Eventually(func() error {
						return f.DeleteLuksSecret(luksSecretName)
					}).Should(Succeed())

					storageClass = originalStorageClass
				})
			})
		})
	})
})
