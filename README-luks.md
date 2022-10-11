The ability to encrypt a PVC with a user owned secret provides an additional security layer that gives control of the data to the cluster owner instead of the platform provider.

## Requirements

1.  Linode CSI driver with LUKS support deployed to your cluster
2.  /tmp tmpfs mount added for csi-linode-node DaemonSet, will otherwise refuse to manage secrets with cryptSetup.
3.  StorageClass with LUKS enabled.

## Notes

1.  Resize is possible with similar steps to resizing PVCs on LKE and are
not handled by driver.  Need cryptSetup resize + resize2fs on LKE node.
2.  Key rotation process is not handled by driver but is possible via similar steps to out of band resize operations.
3.  Encryption is only possible on a new/empty PVC.
4.  LUKS key is currently pulled from a native Kubernetes secret.  Take note of how your cluster handles secrets in etcd.  The CSI driver is careful to otherwise keep the secret on an ephemeral tmpfs mount and otherwise refuses to continue.

## Example StorageClass

```
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
  csi.storage.k8s.io/node-stage-secret-namespace: ${pvc.namespace}
  csi.storage.k8s.io/node-stage-secret-name: ${pvc.name}-luks-key
```


## csi-linode-node DaemonSet change for tmpfs

```
        - mountPath: /tmp
          name: tmpfs
```

## Example PVC.

```
apiVersion: v1
kind: Secret
metadata:
  name: csi-example-pvcluks-luks-key
  namespace: default
stringData:
  luksKey: "SECRETGOESHERE"
---
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
