package framework

import (
	appsv1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetStatefulSetObject(name, namespace, storageClass string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":     name,
				"app.kubernetes.io/instance": name,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: name,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": name,
				},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
					Labels: map[string]string{
						"app.kubernetes.io/name": name,
					},
				},
				Spec: core.PodSpec{
					SecurityContext: &core.PodSecurityContext{
						FSGroup: func(i int64) *int64 { return &i }(1001),
					},
					AutomountServiceAccountToken: func(b bool) *bool { return &b }(false),
					Containers: []core.Container{
						{
							Name:  name,
							Image: "bitnami/redis",
							Env: []core.EnvVar{
								{
									Name:  "ALLOW_EMPTY_PASSWORD",
									Value: "true",
								},
							},
							SecurityContext: &core.SecurityContext{
								RunAsUser: func(i int64) *int64 { return &i }(1001),
							},
							VolumeMounts: []core.VolumeMount{
								{
									Name:      "data",
									MountPath: "/data",
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []core.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "data"},
					Spec: core.PersistentVolumeClaimSpec{
						StorageClassName: &storageClass,
						AccessModes: []core.PersistentVolumeAccessMode{
							core.ReadWriteOnce,
						},
						Resources: core.ResourceRequirements{
							Requests: core.ResourceList{
								core.ResourceName(core.ResourceStorage): resource.MustParse("1Gi"),
							},
						},
					},
				},
			},
		},
	}
}

func (f *Invocation) CreateStatefulSet(sts *appsv1.StatefulSet) error {
	_, err := f.kubeClient.AppsV1().StatefulSets(sts.ObjectMeta.Namespace).Create(sts)
	if err != nil {
		return err
	}
	return nil
}

func (f *Invocation) DeleteStatefulSet(meta metav1.ObjectMeta) error {
	return f.kubeClient.AppsV1().StatefulSets(meta.Namespace).Delete(meta.Name, deleteInForeground())
}

func (f *Invocation) GetStatefulSet(name, namespace string) (*appsv1.StatefulSet, error) {
	return f.kubeClient.AppsV1().StatefulSets(namespace).Get(name, metav1.GetOptions{})
}
