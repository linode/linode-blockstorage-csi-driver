# Linode Block Storage CSI Driver [![Build Status](https://travis-ci.org/linode/linode-blockstorage-csi-driver.svg?branch=master)](https://travis-ci.org/linode/linode-blockstorage-csi-driver)

## Overview
The Container Storage Interface ([CSI](https://github.com/container-storage-interface/spec)) Driver for Linode Block Storage enables container orchestrators such as Kubernetes to manage the life-cycle of persistant storage claims.

More information about the Kubernetes CSI can be found in the GitHub [Kubernetes CSI](https://kubernetes-csi.github.io/docs/Example.html) and [CSI Spec](https://github.com/container-storage-interface/spec/) repos.

## Disclaimer

**Warning**: This driver is a Work-In-Progress and may not be compatible between driver versions and Kubernetes versions.

This is not officially supported by Linode.

## Deployment

### Requirements

* Kubernetes v1.12+
* The node `hostname` must match the Linode Instance `label`
* `--allow-privileged` must be enabled for the API server and kubelet

### Secure a Linode API Access Token:

Generate a Personal Access Token (PAT) using the [Linode Cloud Manager](https://cloud.linode.com/profile/tokens).

This token will need:

* Read/Write access to Volumes (to create and delete volumes)
* Read/Write access to Linodes (to attach and detach volumes)
* A sufficient "Expiry" to allow for continued use of your volume

### Create a Kubernetes secret

Run the following commands to stash a `LINODE_TOKEN` in your Kubernetes cluster:

```bash
read -s -p "Linode API Access Token: " LINODE_TOKEN
read -p "Linode Region of Cluster: " LINODE_REGION
cat <<EOF | kubectl create -f -
```

Paste the following text at the prompt:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: linode
  namespace: kube-system
stringData:
  token: "$LINODE_TOKEN"
  region: "$LINODE_REGION"
EOF
```

You should receive notification that the secret was created.  You can confirm this by running:

```sh
$ kubectl -n kube-system get secrets
NAME                  TYPE                                  DATA      AGE
linode          Opaque                                2         18h
```

### Deploy the CSI

The following command will deploy the CSI and related volume attachment and provisioning sidecars:

```sh
kubectl apply -f https://raw.githubusercontent.com/linode/linode-blockstorage-csi-driver/master/pkg/linode-bs/deploy/releases/linode-blockstorage-csi-driver-v0.0.1.yaml
```

This deployment is a concatenation of all of the `yaml` files in [https://github.com/linode/linode-blockstorage-csi-driver/tree/master/pkg/linode-bs/deploy/kubernetes/].

Notably, this deployment will make `linode-block-storage` the default storageclass.  This behavior can be modified in the [csi-storageclass.yaml](https://github.com/linode/linode-blockstorage-csi-driver/blob/master/pkg/linode-bs/deploy/kubernetes/csi-storageclass.yaml) section of the deployment by toggling the `storageclass.kubernetes.io/is-default-class` annotation.

```sh
$ kubectl get storageclasses
NAME                             PROVISIONER               AGE
linode-block-storage (default)   linodebs.csi.linode.com   2d
```

### Create a PersistentVolumeClaim

Verify that the volume is created, provisioned, mounted, and consumed properlyThis makes sure a volume is created and provisioned on your behalf:

```sh
kubectl create -f https://raw.githubusercontent.com/linode/linode-blockstorage-csi-driver/master/pkg/linode-bs/examples/kubernetes/csi-pvc.yaml
kubectl create -f https://raw.githubusercontent.com/linode/linode-blockstorage-csi-driver/master/pkg/linode-bs/examples/kubernetes/csi-app.yaml
```

Verify that the pod is running and can consume the volume:

```sh
kubectl get pvc
kubectl describe pods/my-csi-app
```

```sh
$ kubectl exec -it my-csi-app /bin/sh
/ # touch /data/hello-world
/ # exit
$ kubectl exec -ti my-csi-app /bin/sh
/ # ls /data
hello-world
```

## Contribution Guidelines

Want to improve the linode-blockstorage-csi-driver? Please start [here](/CONTRIBUTING.md).

## Join the Go Community

For general help or discussion, join the [Gophers Slack team](https://gophers.slack.com) channel [#linodego](https://gophers.slack.com/messages/CAG93EB2S).

