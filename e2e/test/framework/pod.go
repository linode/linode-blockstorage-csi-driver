package framework

import (
	"fmt"
	"strings"

	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kmodules.xyz/client-go/tools/exec"
)

func (f *Invocation) GetPodObject(name, namespace, pvc string, volumeType core.PersistentVolumeMode) (*core.Pod, error) {
	pod := &core.Pod{
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

	switch volumeType {
	case core.PersistentVolumeFilesystem:
		pod.Spec.Containers[0].VolumeMounts = []core.VolumeMount{
			{
				MountPath: "/data",
				Name:      "csi-volume",
			},
		}
	case core.PersistentVolumeBlock:
		pod.Spec.Containers[0].VolumeDevices = []core.VolumeDevice{
			{
				Name:       "csi-volume",
				DevicePath: "/dev/block",
			},
		}
	default:
		return nil, VolumeTypeRequiredError
	}

	return pod, nil
}

func (f *Invocation) CreatePod(pod *core.Pod) error {
	_, err := f.kubeClient.CoreV1().Pods(pod.ObjectMeta.Namespace).Create(f.ctx, pod, metav1.CreateOptions{})
	return err
}

func (f *Invocation) DeletePod(meta metav1.ObjectMeta) error {
	err := f.kubeClient.CoreV1().Pods(meta.Namespace).Delete(f.ctx, meta.Name, deleteInForeground())
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (f *Invocation) GetPod(name, namespace string) (*core.Pod, error) {
	return f.kubeClient.CoreV1().Pods(namespace).Get(f.ctx, name, metav1.GetOptions{})
}

func (f *Invocation) IsPodReady(meta metav1.ObjectMeta) error {
	pod, err := f.kubeClient.CoreV1().Pods(meta.Namespace).Get(f.ctx, meta.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if pod.Status.Phase == core.PodRunning {
		return nil
	}
	return fmt.Errorf("pod %s/%s not ready: %v", meta.Namespace, meta.Name, pod.Status.Phase)
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
	if out == filename || err == nil {
		return nil
	}

	return fmt.Errorf("file name %v not found: %w", filename, err)
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
