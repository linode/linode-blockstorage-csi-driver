## 🌐 Topology-Aware Provisioning

This CSI driver supports topology-aware provisioning, optimizing volume placement based on the physical infrastructure layout.

**Notes:**

1. **Volume Cloning**: Cloning only works within the same region, not across regions.
2. **Volume Migration**: We can't move volumes across regions.
3. **Remote Provisioning**: Volume provisioning is supported in remote regions (nodes or clusters outside of the region where the controller server is deployed).

> [!IMPORTANT]
> Make sure you are using the latest release v0.8.6+ to utilize the remote provisioning feature.

#### 📝 Example StorageClass and PVC

```yaml
allowVolumeExpansion: true
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: linode-block-storage-wait-for-consumer
provisioner: linodebs.csi.linode.com
reclaimPolicy: Delete
volumeBindingMode: WaitForFirstConsumer
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: pvc-filesystem
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: linode-block-storage-wait-for-consumer
```

> **Important**: The `volumeBindingMode: WaitForFirstConsumer` setting is crucial for topology-aware provisioning. It delays volume binding and creation until a pod using the PVC is created. This allows the system to consider the pod's scheduling requirements and node assignment when selecting the most appropriate storage location, ensuring optimal data locality and performance.

#### 🖥️ Example Pod

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: e2e-pod
spec:
  nodeSelector:
    topology.linode.com/region: us-ord
  tolerations:
  - key: "node-role.kubernetes.io/control-plane"
    operator: "Exists"
    effect: "NoSchedule"
  containers:
  - name: e2e-pod
    image: ubuntu
    command:
    - sleep
    - "1000000"
    volumeMounts:
    - mountPath: /data
      name: csi-volume
  volumes:
  - name: csi-volume
    persistentVolumeClaim:
      claimName: pvc-filesystem
```

This example demonstrates how to set up topology-aware provisioning using the Linode Block Storage CSI Driver. The StorageClass defines the provisioner and reclaim policy, while the PersistentVolumeClaim requests storage from this class. The Pod specification shows how to use the PVC and includes a node selector for region-specific deployment.

> [!IMPORTANT]
> To enable topology-aware provisioning, make sure to pass the following argument to the csi-provisioner sidecar:
> ```
> --feature-gates=CSINodeInfo=true
> ```
> This enables the CSINodeInfo feature gate, which is required for topology-aware provisioning to function correctly.
> 
> Note: This feature is enabled by default in release v0.8.6 and later versions.

#### Provisioning Process

1. CO (Kubernetes) determines required topology based on application needs (pod scheduled region) and cluster layout.
2. external-provisioner gathers topology requirements from CO and includes `TopologyRequirement` in `CreateVolume` call.
3. CSI driver creates volume satisfying topology requirements.
4. Driver returns actual topology of created volume.

By leveraging topology-aware provisioning, CSI drivers ensure optimal volume placement within the infrastructure, improving performance, availability, and data locality.
