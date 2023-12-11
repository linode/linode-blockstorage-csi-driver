package framework

import (
	"fmt"
	"strings"

	"github.com/appscode/go/wait"
	"github.com/pkg/errors"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kmodules.xyz/client-go/tools/exec"
)

func GetPodObject(name, namespace, pvc string, volumeType core.PersistentVolumeMode) *core.Pod {
	pod := core.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: core.PodSpec{
			Containers: []core.Container{
				{
					Name:    name,
					Image:   "ubuntu",
					Command: []string{"sleep", "1000000"},
				},
			},
			Volumes: []core.Volume{
				{
					Name: "csi-volume",
					VolumeSource: core.VolumeSource{
						PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
							ClaimName: pvc,
						},
					},
				},
			},
		},
	}

	if volumeType == core.PersistentVolumeFilesystem {
		pod.Spec.Containers[0].VolumeMounts = []core.VolumeMount{
			{
				MountPath: "/data",
				Name:      "csi-volume",
			},
		}
	}

	if volumeType == core.PersistentVolumeBlock {
		pod.Spec.Containers[0].VolumeDevices = []core.VolumeDevice{
			{
				Name:       "csi-volume",
				DevicePath: "/dev/block",
			},
		}
	}

	return &pod
}

func (f *Invocation) GetPodObjectWithBlockVolume(pvc string) *core.Pod {
	return &core.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.app,
			Namespace: f.namespace,
		},
		Spec: core.PodSpec{
			Containers: []core.Container{
				{
					Name:  f.app,
					Image: "ubuntu",
					VolumeDevices: []core.VolumeDevice{
						{
							DevicePath: "/dev/block",
							Name:       "csi-volume",
						},
					},
					Command: []string{"sleep", "1000000"},
				},
			},
			Volumes: []core.Volume{
				{
					Name: "csi-volume",
					VolumeSource: core.VolumeSource{
						PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
							ClaimName: pvc,
						},
					},
				},
			},
		},
	}
}

func (f *Invocation) CreatePod(pod *core.Pod) error {
	pod, err := f.kubeClient.CoreV1().Pods(pod.ObjectMeta.Namespace).Create(pod)
	if err != nil {
		return err
	}
	return f.WaitForReady(pod.ObjectMeta)

}

func (f *Invocation) DeletePod(meta metav1.ObjectMeta) error {
	err := f.kubeClient.CoreV1().Pods(meta.Namespace).Delete(meta.Name, deleteInForeground())
	if err != nil && apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (f *Invocation) GetPod(name, namespace string) (*core.Pod, error) {
	return f.kubeClient.CoreV1().Pods(namespace).Get(name, metav1.GetOptions{})
}

func (f *Invocation) WaitForReady(meta metav1.ObjectMeta) error {
	return wait.PollImmediate(f.RetryInterval, f.Timeout, func() (bool, error) {
		pod, err := f.kubeClient.CoreV1().Pods(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
		if pod == nil || err != nil {
			return false, nil
		}
		if pod.Status.Phase == core.PodRunning {
			return true, nil
		}
		return false, nil
	})
}

func (f *Invocation) WaitForDelete(meta metav1.ObjectMeta) error {
	return wait.PollImmediate(f.RetryInterval, f.Timeout, func() (bool, error) {
		_, err := f.kubeClient.CoreV1().Pods(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
}

func (f *Invocation) WriteFileIntoPod(filename string, pod *core.Pod) error {
	_, err := exec.ExecIntoPod(f.restConfig, pod, exec.Command([]string{
		"touch", filename,
	}...))

	return err
}

func (f *Invocation) CheckIfFileIsInPod(filename string, pod *core.Pod) error {
	out, err := exec.ExecIntoPod(f.restConfig, pod, exec.Command([]string{
		"ls", filename,
	}...))
	if out == filename {
		return nil
	}
	return errors.Wrap(err, fmt.Sprintf("file name %v not found", filename))
}

func (f *Invocation) MkfsInPod(pod *core.Pod) error {
	_, err := exec.ExecIntoPod(f.restConfig, pod, exec.Command([]string{
		"mkfs.ext4", "-b", "2048", "-L", "csi-volume", "-F", "/dev/block",
	}...))

	// mkfs.ext4 outputs "mke2fs 1.46.5 (30-Dec-2021)" on stderr for some reason even though it makes the filesystem
	// since this hasn't changed in 2yrs I will hard code this but note if the ubuntu container gets a new version someday
	// this will need to be updated
	if strings.Contains(err.Error(), "mke2fs 1.46.5 (30-Dec-2021)") {
		return nil
	}

	return err
}

func (f *Invocation) MountDriveInPod(pod *core.Pod) error {
	out, err := exec.ExecIntoPod(f.restConfig, pod, exec.Command([]string{
		"mkdir", "-p", "/data", "&&", "mount", "/dev/block", "/data", "-o bind",
	}...))

	fmt.Printf("out: %s, error: %s", out, err)
	return err
}
