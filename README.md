# Linode Block Storage CSI Driver
[![Go Report Card](https://goreportcard.com/badge/github.com/linode/linode-blockstorage-csi-driver)](https://goreportcard.com/report/github.com/linode/linode-blockstorage-csi-driver)
[![codecov](https://codecov.io/gh/linode/linode-blockstorage-csi-driver/graph/badge.svg?token=b5HeEgMdAd)](https://codecov.io/gh/linode/linode-blockstorage-csi-driver)
[![Docker Pulls](https://img.shields.io/docker/pulls/linode/linode-blockstorage-csi-driver.svg)](https://hub.docker.com/r/linode/linode-blockstorage-csi-driver/)

## Overview
The Container Storage Interface
([CSI](https://github.com/container-storage-interface/spec)) Driver for Linode
Block Storage enables container orchestrators such as Kubernetes to manage the
life-cycle of persistent storage claims.

More information about the Kubernetes CSI can be found in the GitHub [Kubernetes
CSI](https://kubernetes-csi.github.io/docs/example.html) and [CSI
Spec](https://github.com/container-storage-interface/spec/) repositories.

## Deployment

There are two ways of deploying CSI.
The recommended way is to use Helm.
The second way is to use manually set secrets on the kubernetes cluster then use
kubectl to deploy using the given [`yaml`
file](https://raw.githubusercontent.com/linode/linode-blockstorage-csi-driver/master/pkg/linode-bs/deploy/releases/linode-blockstorage-csi-driver.yaml). 

### Requirements
* Kubernetes v1.16+
* [LINODE_API_TOKEN](https://cloud.linode.com/profile/tokens) (See 'Secure a
  Linode API Access Token' below).
* Linode [REGION](https://api.linode.com/v4/regions).


### Secure a Linode API Access Token:
Generate a Personal Access Token (PAT) using the [Linode Cloud
Manager](https://cloud.linode.com/profile/tokens).

This token will need:

* Read/Write access to Volumes (to create and delete volumes)
* Read/Write access to Linodes (to attach and detach volumes)
* A sufficient "Expiry" to allow for continued use of your volumes

### [Recommended] Using Helm to deploy the CSI Driver

`LINODE_API_TOKEN` must be a Linode APIv4 [Personal Access
Token](https://cloud.linode.com/profile/tokens) with Volume and Linode
permissions.

`REGION` must be a Linode [region](https://api.linode.com/v4/regions).

#### Install the csi-linode repo

```sh
helm repo add linode-csi https://linode.github.io/linode-blockstorage-csi-driver/   
helm repo update linode-csi
```

#### To deploy the CSI Driver

```sh
export LINODE_API_TOKEN="...your Linode API token..."
export REGION="your preferred region"

helm install linode-csi-driver \
  --set apiToken="${LINODE_API_TOKEN}" \
  --set region="${REGION}" \
  linode-csi/linode-blockstorage-csi-driver
```
_See [helm install](https://helm.sh/docs/helm/helm_install/) for command documentation._

#### Uninstalling linode-blockstorage-csi-driver

To uninstall the linode-blockstorage-csi-driver from your cluster, run the
following command:

```sh
helm uninstall linode-csi-driver
```
_See [helm uninstall](https://helm.sh/docs/helm/helm_uninstall/) for command
documentation._

#### Upgrading linode-blockstorage-csi-driver

To upgrade when new changes are made to the helm chart, run the following
command:

```sh
export LINODE_API_TOKEN="...your Linode API token..."
export REGION="your preferred region"

helm upgrade linode-csi-driver \
  --install \
  --set apiToken="${LINODE_API_TOKEN}" \
  --set region="${REGION}" \
  linode-csi/linode-blockstorage-csi-driver
```
_See [helm upgrade](https://helm.sh/docs/helm/helm_upgrade/) for command
documentation._

#### Configurations

There are other variables that can be set to a different value.
For list of all the modifiable variables/values, see
[helm-chart/csi-driver/values.yaml](https://github.com/linode/linode-blockstorage-csi-driver/blob/main/helm-chart/csi-driver/values.yaml).

Values can be set/overrided by using the '--set var=value,...' flag or by
passing in a custom-values.yaml using '-f custom-values.yaml'.

Recommendation: Use a custom `values.yaml` file to override the variables to
avoid any errors with template rendering.

### Deploying with kubectl

#### Create a secret

Create the following secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: linode
  namespace: kube-system
stringData:
  token: "your linode api token"
  region: "your preferred region"
```

then apply it with 

```sh
kubectl apply -f secret.yaml
```

You should receive notification that the secret was created.
You can confirm this by running:

```sh
$ kubectl -n kube-system get secret/linode
NAME    TYPE      DATA      AGE
linode  Opaque    2         2m
```

#### Deploy the CSI driver

The following command will deploy the latest version of the CSI driver with all
related Kubernetes volume attachment, driver registration, and provisioning
sidecars:

```sh
kubectl apply -f https://raw.githubusercontent.com/linode/linode-blockstorage-csi-driver/master/pkg/linode-bs/deploy/releases/linode-blockstorage-csi-driver.yaml
```

If you need a specific
[release](https://github.com/linode/linode-blockstorage-csi-driver/releases) of
the CSI driver you can specify the release version in the URL like the
following:

```sh
kubectl apply -f https://raw.githubusercontent.com/linode/linode-blockstorage-csi-driver/master/pkg/linode-bs/deploy/releases/linode-blockstorage-csi-driver-v0.5.3.yaml
```

This deployment is a concatenation of all of the `yaml` files in
[pkg/linode-bs/deploy/kubernetes/](https://github.com/linode/linode-blockstorage-csi-driver/tree/master/pkg/linode-bs/deploy/kubernetes/).

Notably, this deployment will:

* Set the default storage class to `linode-block-storage-retain` [Learn
  More](https://kubernetes.io/docs/tasks/administer-cluster/change-default-storage-class/)

  This behavior can be modified in the
  [csi-storageclass.yaml](https://github.com/linode/linode-blockstorage-csi-driver/blob/master/pkg/linode-bs/deploy/kubernetes/05-csi-storageclass.yaml)
  section of the deployment by toggling the
  `storageclass.kubernetes.io/is-default-class` annotation.

  ```sh
  $ kubectl get storageclasses
  NAME                                    PROVISIONER               AGE
  linode-block-storage-retain (default)   linodebs.csi.linode.com   2d
  linode-block-storage                    linodebs.csi.linode.com   2d
  ```

* Use a `reclaimPolicy` of `Retain` [Learn
  More](https://kubernetes.io/docs/tasks/administer-cluster/change-pv-reclaim-policy/)

  Volumes created by this CSI driver will default to using the
  `linode-block-storage-retain` storage class if one is not specified.
  Upon deletion of all PersitentVolumeClaims, the PersistentVolume and its
  backing Block Storage Volume will remain intact.

* Assume that the [Linode
  CCM](https://github.com/linode/linode-cloud-controller-manager) is initialized
  and running [Learn More](https://kubernetes.io/docs/reference/command-line-tools-reference/cloud-controller-manager/)

  If you absolutely intend to run this on a cluster which will not run the
  Linode CCM, you must modify the init container script located in the
  `08-cm-get-linode-id.yaml` ConfigMap and delete [the
  line](https://github.com/linode/linode-blockstorage-csi-driver/blob/master/pkg/linode-bs/deploy/kubernetes/08-cm-get-linode-id.yaml#L19)
  that contains the `exit 1`.

### Example Usage

This repository contains [two
manifests](https://github.com/linode/linode-blockstorage-csi-driver/tree/master/pkg/linode-bs/examples/kubernetes)
that demonstrate use of the Linode BlockStorage CSI.
These manifests will create a PersistentVolume Claim using the
`linode-block-storage-retain` storage class and then consume it in a minimal
pod.

Once you have installed the Linode BlockStorage CSI, the following commands will
run the example.
Once you are finished with the example, please be sure to delete the pod, PVC,
and the associated Block Storage Volume.
The PVC created in this example uses the `linode-block-storage-retain` storage
class, so you will need to remove the Block Storage Volume from your Linode
account via the Cloud Manager or the Linode CLI.

```sh
kubectl create -f https://raw.githubusercontent.com/linode/linode-blockstorage-csi-driver/master/pkg/linode-bs/examples/kubernetes/csi-pvc.yaml
kubectl create -f https://raw.githubusercontent.com/linode/linode-blockstorage-csi-driver/master/pkg/linode-bs/examples/kubernetes/csi-app.yaml
```

Verify that the pod is running and can consume the volume:

```sh
kubectl get pvc/csi-example-pvc pods/csi-example-pod
kubectl describe pvc/csi-example-pvc pods/csi-example-pod
```

Now, let's add some data into the PVC, and delete and recreate the pod.
Our data will remain intact.

First, we will write some data to a file in the PVC:

```sh
$ kubectl exec -it csi-example-pod -- /bin/sh -c "echo persistence > /data/example.txt; ls -l /data"
total 20
-rw-r--r--    1 root     root            12 Dec  5 13:06 example.txt
drwx------    2 root     root         16384 Dec  5 06:03 lost+found
```

Then we will delete and recreate the pod, and check to make sure our data is
still there:

```sh
$ kubectl delete pods/csi-example-pod
pod "csi-example-pod" deleted

$ kubectl create -f https://raw.githubusercontent.com/linode/linode-blockstorage-csi-driver/master/pkg/linode-bs/examples/kubernetes/csi-app.yaml
pod/csi-example-pod created

$ sleep 30; kubectl exec -it csi-example-pod -- /bin/sh -c "ls -l /data; cat /data/example.txt"
total 20
-rw-r--r--    1 root     root            12 Dec  5 13:06 example.txt
drwx------    2 root     root         16384 Dec  5 06:03 lost+found
persistence

```

### Encrypted Drives using LUKS

The ability to encrypt a PVC with a user owned secret provides an additional
security layer that gives control of the data to the cluster owner instead of
the platform provider.

#### Notes

1.  Resize is possible with similar steps to resizing PVCs on LKE and are
    not handled by driver.  Need cryptSetup resize + resize2fs on LKE node.
2.  Key rotation process is not handled by driver but is possible via similar
    steps to out of band resize operations.
3.  Encryption is only possible on a new/empty PVC.
4.  LUKS key is currently pulled from a native Kubernetes secret.
    Take note of how your cluster handles secrets in etcd.
    The CSI driver is careful to otherwise keep the secret on an ephemeral tmpfs
    mount and otherwise refuses to continue.

#### Example StorageClass

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

#### Example PVC

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
### Adding Tags to created volumes

This feature gives users the ability to add tags to volumes created by a
specific storageClass, to allow for better tracking of volumes.
Tags are added as a comma seperated string value for a parameter
`linodebs.csi.linode.com/volumeTags`

#### Example StorageClass

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
    linodebs.csi.linode.com/volumeTags: "test, foo, yolo"
```


## Disclaimers

* Until this driver has reached v1.0.0 it may not maintain compatibility between
  driver versions.
* Requests for Persistent Volumes with a `require_size` less than the Linode
  minimum Block Storage size will be fulfilled with a Linode Block Storage volume
  of the minimum size (currently `10Gi`).
  This is [in accordance with the CSI
  specification](https://github.com/container-storage-interface/spec/blob/v1.0.0/spec.md#createvolume).
  The upper-limit size constraint (`limit_bytes`) will also be honored so the
  size of Linode Block Storage volumes provisioned will not exceed this
  parameter.

## Contribution Guidelines

Want to improve the linode-blockstorage-csi-driver?
Please start [here](.github/CONTRIBUTING.md).

## Join us on Slack

For general help or discussion, join the [Kubernetes
Slack](http://slack.k8s.io/) channel
[#linode](https://kubernetes.slack.com/messages/CD4B15LUR).

For development and debugging, join the [Gopher's
Slack](https://invite.slack.golangbridge.org/) channel
[#linodego](https://gophers.slack.com/messages/CAG93EB2S).
