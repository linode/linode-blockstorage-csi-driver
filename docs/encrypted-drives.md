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
