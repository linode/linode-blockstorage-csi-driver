## 🔒 Encrypted Drives

**Notes**:

1. **Setting Up Encryption**: In the provided StorageClasses, encryption is activated by specifying `linodebs.csi.linode.com/encrypted: "true"` in the `parameters` field. This signals the CSI driver to provision volumes with encryption enabled, provided encryption is supported in the specified region.
2. **Retention and Expansion Options**:
    - The `linode-block-storage-encrypted` StorageClass uses the default `Delete` reclaim policy, meaning that volumes created with this StorageClass will be deleted when the associated PVC is deleted.
    - In contrast, the `linode-block-storage-retain-encrypted` StorageClass uses the `Retain` policy. This allows the volume to persist even after the PVC is deleted, ensuring data is preserved until manually removed.
    - Both StorageClasses support volume expansion through the `allowVolumeExpansion: true` setting, allowing users to resize volumes as needed without data loss.

3. **Default StorageClass Annotation**: By marking both StorageClasses with `storageclass.kubernetes.io/is-default-class: "true"`, they’re eligible to act as default classes. However, Kubernetes will only treat one StorageClass as the actual default. Consider applying this annotation only to the preferred default StorageClass.
4. **Region Compatibility**: Ensure that encryption is supported in the Linode region where the volumes will be created. If encryption is not available in a specific region, the CSI driver will return an error.
5. **Usage in PersistentVolumeClaims (PVCs)**: Use the `storageClassName` field in a PVC to reference the desired StorageClass (`linode-block-storage-encrypted` or `linode-block-storage-retain-encrypted`). Each PVC will inherit the encryption settings defined in the referenced StorageClass.

#### 🔑 Example StorageClass

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: linode-block-storage-encrypted
  namespace: kube-system
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
parameters:
  linodebs.csi.linode.com/encrypted: "true"
allowVolumeExpansion: true
provisioner: linodebs.csi.linode.com
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: linode-block-storage-retain-encrypted
  namespace: kube-system
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
parameters:
  linodebs.csi.linode.com/encrypted: "true"
allowVolumeExpansion: true
provisioner: linodebs.csi.linode.com
reclaimPolicy: Retain
```

#### 📝 Example PVC

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: csi-example-pvc-encrypted
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: linode-block-storage-encrypted
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: csi-example-pvc-encrypted-retain
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: linode-block-storage-retain-encrypted
```


---

## 🔒 Encrypted Drives using LUKS

**Notes:**

1. **Resizing**: Resize is possible with similar steps to resizing PVCs on LKE and are
    not handled by driver.  Need cryptSetup resize + resize2fs on LKE node.
2. **Key Rotation**: Key rotation process is not handled by driver but is possible via similar
    steps to out of band resize operations.
3. **PVC Requirement**: Encryption only possible on a new/empty PVC.
4. **Secret Handling**: LUKS key is currently pulled from a native Kubernetes secret.
    Take note of how your cluster handles secrets in etcd.
    The CSI driver is careful to otherwise keep the secret on an ephemeral tmpfs
    mount and otherwise refuses to continue.

#### 🔑 Example StorageClass

> [!TIP]
> To use an encryption key per PVC you can make a new StorageClass/Secret
> combination each time.

```yaml
allowVolumeExpansion: true
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
  name: linode-block-storage-retain-luks
  namespace: kube-system
provisioner: linodebs.csi.linode.com
reclaimPolicy: Retain
parameters:
  linodebs.csi.linode.com/luks-encrypted: "true"
  linodebs.csi.linode.com/luks-cipher: "aes-xts-plain64"
  linodebs.csi.linode.com/luks-key-size: "512"
  csi.storage.k8s.io/node-stage-secret-namespace: csi-encrypt-example
  csi.storage.k8s.io/node-stage-secret-name: csi-encrypt-example-luks-key
---
apiVersion: v1
kind: Secret
metadata:
  name: csi-encrypt-example-luks-key
  namespace: csi-encrypt-example
stringData:
  luksKey: "SECRETGOESHERE"  
```

#### 📝 Example PVC

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: csi-example-pvc
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: linode-block-storage-retain
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: csi-example-pvcluks
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: linode-block-storage-retain-luks
```
