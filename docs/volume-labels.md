---
nav_order: 5
---

# Naming Volumes After Their PVC

By default a dynamically provisioned volume is labeled with the name the
external-provisioner generates for it, for example `pvc-72af5c8ff02c4813`
(plus `LINODE_VOLUME_LABEL_PREFIX`, if set). That label does not say which
workload owns the volume in Cloud Manager, the API or billing exports.

Set `linodebs.csi.linode.com/volumeLabelFromPVC: "true"` on a StorageClass to
label new volumes after the PVC instead:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: linode-block-storage-named
provisioner: linodebs.csi.linode.com
allowVolumeExpansion: true
parameters:
  linodebs.csi.linode.com/volumeLabelFromPVC: "true"
```

A PVC `data-postgres-0` in namespace `default` then gets the label
`default-data-postgres-0-f02c4813`.

## How the label is built

`<prefix><namespace>-<pvc name>-<suffix>`

- `prefix` is `LINODE_VOLUME_LABEL_PREFIX` (empty by default).
- `suffix` is the last 8 characters of the generated volume name, which come
  from the PVC's UID. It keeps labels unique when long names are shortened and
  when a PVC is deleted and recreated with the same name while the old volume
  is still around (for example with `reclaimPolicy: Retain`).
- Linode volume labels are limited to 32 characters. When the label would be
  longer, characters are removed from the middle of `<namespace>-<pvc name>`,
  so the namespace at the start and the end of the PVC name (such as a
  StatefulSet ordinal) are kept.
- Characters Linode does not allow in labels (such as `.`) become `-`, and
  repeated `-` or `_` are collapsed.

The label is chosen once, when the volume is created, and it is stored in the
volume handle. Do not rename volumes managed by the driver afterwards: the
node plugin finds the attached device by the label in the handle, so a renamed
volume will fail to mount the next time it is attached.

## Requirements and fallback

The external-provisioner must run with `--extra-create-metadata` so the PVC
name and namespace are passed to the driver. The Helm chart and the kustomize
manifests in this repository set it. If the metadata is missing, or no valid
label can be built (for example a namespace that starts with a digit and no
label prefix is set), the driver logs this and uses the default label.

Existing volumes are not changed. The parameter only affects volumes created
after it is set.
