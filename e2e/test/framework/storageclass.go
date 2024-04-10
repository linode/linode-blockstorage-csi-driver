package framework

import (
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const driverName = "linodebs.csi.linode.com"

func (f *Framework) CreateStorageClass(sc *v1.StorageClass) error {
	_, err := f.kubeClient.StorageV1().StorageClasses().Create(sc)

	return err
}

// DeleteStorageClass returns a storage class that can be subsequently created. The parameters can be obtained using GetLuksParameters
func (f *Framework) DeleteStorageClass(name string) error {
	return f.kubeClient.StorageV1().StorageClasses().Delete(name, &metav1.DeleteOptions{})
}

// GetStorageClass returns a storage class that can be subsequently created. The parameters can be obtained using GetLuksParameters
func (f *Framework) GetStorageClass(name string, parameters map[string]string) *v1.StorageClass {
	return &v1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner:          driverName,
		Parameters:           parameters,
		ReclaimPolicy:        ptrT(corev1.PersistentVolumeReclaimDelete),
		AllowVolumeExpansion: ptrT(true),
	}
}

func (f *Framework) GetLuksParameters(secretName, secretNamespace string) map[string]string {
	return map[string]string{
		"linodebs.csi.linode.com/luks-encrypted":         "true",
		"linodebs.csi.linode.com/luks-cipher":            "aes-xts-plain64",
		"linodebs.csi.linode.com/luks-key-size":          "512",
		"csi.storage.k8s.io/node-stage-secret-namespace": secretNamespace,
		"csi.storage.k8s.io/node-stage-secret-name":      secretName,
	}
}

// CreateLuksSecret creates a secret containing a valid  LUKS key. The namespace is optional and defaults to kube-system as the secret must be readable by the CSI controller.
// TODO: Currently the key is static but we could and should generate them dynamically.
func (f *Framework) CreateLuksSecret(name string, namespace ...string) error {
	ns := "kube-system"
	if len(namespace) > 0 {
		ns = namespace[0]
	}

	_, err := f.kubeClient.CoreV1().Secrets(ns).Create(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Data: map[string][]byte{
			"luksKey": []byte("aERFS0ZnRVpnbXB1cHBTaFBHN0hhaWxTRkJzeThNemx2bGhBTHZxazArMmpUcmNLckZtdHR0b0Y1SUdsTFZvTHQvanBhV25rL2tjbDdKeG5zWjN4UWpFY1l1bXY0V2t3T3Y3N3grYzJDL2t5eWxkVE5SYUNhVkhHOWZXOW42b2ljb1d6c3lVV2NtdTBkK0pPb3JHWjc5MmxzUzlRNWdYbENnNUJEMngxTW9WVnI4aFRRQXJGZlVYNk51SEYxbzB2L0VHSFUwQTVPNXdpTm5xcGREamY5cjU2clB0MEgyOTBOcjZZNUlqYjVSVElvSkZUNXd3NVhvY3J2TGxSL0dpWFJZZ3plSVNmYmZ5SXI4RnBmUkttalBUWmRMQlNYUE1NZEhKTmNQSWxSRytEZm5CYVRLa0lGd2lXWGp4WFpzczcxSUtpYkVNN1FmandrYTBLRnl1ZndBPT0="),
		},
		StringData: map[string]string{},
		Type:       "",
	})

	return err
}

func (f *Framework) DeleteLuksSecret(name string, namespace ...string) error {
	ns := "kube-system"
	if len(namespace) > 0 {
		ns = namespace[0]
	}

	return f.kubeClient.CoreV1().Secrets(ns).Delete(name, &metav1.DeleteOptions{})
}
