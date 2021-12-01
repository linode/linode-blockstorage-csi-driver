package framework

import (
	"context"
	"strconv"
	"strings"

	"github.com/linode/linodego"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *Invocation) GetPersistentVolumeClaimObject(size, storageClass string) *core.PersistentVolumeClaim {
	return &core.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.app,
			Namespace: f.namespace,
		},
		Spec: core.PersistentVolumeClaimSpec{
			AccessModes: []core.PersistentVolumeAccessMode{
				core.ReadWriteOnce,
			},
			StorageClassName: &storageClass,
			Resources: core.ResourceRequirements{
				Requests: core.ResourceList{
					core.ResourceName(core.ResourceStorage): resource.MustParse(size),
				},
			},
		},
	}
}

func (f *Invocation) GetPersistentVolumeClaim(meta metav1.ObjectMeta) (*core.PersistentVolumeClaim, error) {
	return f.kubeClient.CoreV1().PersistentVolumeClaims(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
}

func (f *Invocation) UpdatePersistentVolumeClaim(pvc *core.PersistentVolumeClaim) error {
	_, err := f.kubeClient.CoreV1().PersistentVolumeClaims(pvc.Namespace).Update(pvc)
	return err
}

func (f *Invocation) CreatePersistentVolumeClaim(pvc *core.PersistentVolumeClaim) error {
	_, err := f.kubeClient.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(pvc)
	return err
}

func (f *Invocation) DeletePersistentVolumeClaim(meta metav1.ObjectMeta) error {
	return f.kubeClient.CoreV1().PersistentVolumeClaims(meta.Namespace).Delete(meta.Name, nil)
}

func (f *Invocation) GetVolumeSize(pvc *core.PersistentVolumeClaim) (int, error) {
	pv, err := f.kubeClient.CoreV1().PersistentVolumes().Get(pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return -1, err
	}

	volumeHandle := pv.Spec.CSI.VolumeHandle
	volumeID, err := strconv.Atoi(strings.Split(volumeHandle, "-")[0])
	if err != nil {
		return -1, err
	}
	volume, err := f.linodeClient.GetVolume(context.Background(), volumeID)
	if err != nil {
		return -1, err
	}
	return volume.Size, err
}

func (f *Invocation) GetVolumeID(pvc *core.PersistentVolumeClaim) (int, error) {
	pv, err := f.kubeClient.CoreV1().PersistentVolumes().Get(pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return -1, err
	}

	volumeHandle := pv.Spec.CSI.VolumeHandle
	volumeID, err := strconv.Atoi(strings.Split(volumeHandle, "-")[0])
	if err != nil {
		return -1, err
	}
	return volumeID, err
}

func (f *Invocation) IsVolumeDetached(volumeID int) (bool, error) {
	volume, err := f.linodeClient.GetVolume(context.Background(), volumeID)
	if err != nil {
		return false, err
	}
	return volume.LinodeID == nil, err
}

func (f *Invocation) IsVolumeDeleted(volumeID int) (bool, error) {
	_, err := f.linodeClient.GetVolume(context.Background(), volumeID)
	originalErr, ok := err.(*linodego.Error)
	if ok && originalErr.Code == 404 {
		return true, nil
	}
	return false, err
}
