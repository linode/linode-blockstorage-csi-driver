---
nav_order: 5
---

# 🏷️ Adding Tags to Created Volumes

Add tags to volumes for better tracking by specifying the `linodebs.csi.linode.com/volumeTags` parameter.

## 🔑 Example StorageClass with Tags

```yaml
allowVolumeExpansion: true
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
  name: linode-block-storage
  namespace: kube-system
provisioner: linodebs.csi.linode.com
parameters:
  linodebs.csi.linode.com/volumeTags: "foo, bar"
```
