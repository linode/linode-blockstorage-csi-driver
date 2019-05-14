package framework

import (
	"fmt"

	"github.com/appscode/go/wait"
	"github.com/pkg/errors"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kmodules.xyz/client-go/tools/exec"
)

func (f *Invocation) GetPodObject(name string, pvc string) *core.Pod {
	return &core.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: f.namespace,
		},
		Spec: core.PodSpec{
			Containers: []core.Container{
				{
					Name:  name,
					Image: "busybox",
					VolumeMounts: []core.VolumeMount{
						{
							MountPath: "/data",
							Name:      "csi-volume",
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
	pod, err := f.kubeClient.CoreV1().Pods(f.namespace).Create(pod)
	if err != nil {
		return err
	}
	return f.WaitForReady(pod.ObjectMeta)

}

func (f *Invocation) DeletePod(name string) error {
	return f.kubeClient.CoreV1().Pods(f.namespace).Delete(name, deleteInForeground())
}

func (f *Invocation) GetPod(name, ns string) (*core.Pod, error) {
	return f.kubeClient.CoreV1().Pods(ns).Get(name, metav1.GetOptions{})
}

func (f *Invocation) WaitForReady(meta metav1.ObjectMeta) error {
	return wait.PollImmediate(retryInterval, retryTimout, func() (bool, error) {
		pod, err := f.kubeClient.CoreV1().Pods(f.namespace).Get(meta.Name, metav1.GetOptions{})
		if pod == nil || err != nil {
			return false, nil
		}
		if pod.Status.Phase == core.PodRunning {
			return true, nil
		}
		return false, nil
	})
}

func (f *Invocation) WriteFileIntoPod(filename string, pod *core.Pod) error {
	_, err := exec.ExecIntoPod(f.restConfig, pod, exec.Command([]string{
		"touch", filename,
	}...))

	return err
}

func (f *Invocation) CheckFileIntoPod(filename string, pod *core.Pod) error {
	out, err := exec.ExecIntoPod(f.restConfig, pod, exec.Command([]string{
		"ls", filename,
	}...))
	if out == filename {
		return nil
	}
	return errors.Wrap(err, fmt.Sprintf("file name %v not found", filename))
}
