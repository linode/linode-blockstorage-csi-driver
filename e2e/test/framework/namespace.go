package framework

import (
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *Framework) Namespace() string {
	return f.namespace
}

func (f *Framework) CreateNamespace() error {
	obj := &core.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: f.namespace,
		},
	}
	_, err := f.kubeClient.CoreV1().Namespaces().Create(f.ctx, obj, metav1.CreateOptions{})
	return err
}

func (f *Framework) DeleteNamespace(name string) error {
	return f.kubeClient.CoreV1().Namespaces().Delete(f.ctx, name, deleteInForeground())
}
