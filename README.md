# csi-linode [![Build Status](https://travis-ci.org/displague/csi-linode.svg?branch=master)](https://travis-ci.org/displague/csi-linode)

The Container Storage Interface ([CSI](https://github.com/container-storage-interface/spec)) Driver for Linode Block Storage enables container orchestrators such as Kubernetes to create and mount volumes for persistant storage claims.

More information about the Kubernetes CSI can be found in the GitHub [Kubernetes CSI](https://kubernetes-csi.github.io/docs/Example.html) and [CSI Spec](https://github.com/container-storage-interface/spec/) repos.

## Deployment

### Requirements

* Kubernetes v1.10 or newer
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

```
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
kubectl apply -f https://raw.githubusercontent.com/displague/csi-linode/master/hack/deploy/releases/csi-linode-v0.0.1.yaml
```

This deployment is a concatenation of all of the `yaml` files in [https://github.com/displague/csi-linode/tree/master/hack/deploy].

Notably, this deployment will make linode-block-storage the default storageclass.  This behavior can be modified in the [csi-storageclass.yaml](https://github.com/displague/csi-linode/blob/master/hack/deploy/csi-storageclass.yaml) section of the deployment by toggling the `storageclass.kubernetes.io/is-default-class` annotation.

```sh
$ kubectl get storageclasses
NAME                             PROVISIONER               AGE
linode-block-storage (default)   com.linode.csi.linodebs   2d
```

### Create a PersistentVolumeClaim

Verify that the volume is created, provisioned, mounted, and consumed properlyThis makes sure a volume is created and provisioned on your behalf:

```
kubectl create -f https://github.com/displague/csi-linode/blob/master/hack/deploy/example/csi-pvc.yaml
kubectl create -f https://github.com/displague/csi-linode/blob/master/hack/deploy/example/csi-app.yaml
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

## Discussion / Help

Join us at [#linodego](https://gophers.slack.com/messages/CAG93EB2S) on the [gophers slack](https://gophers.slack.com)

## License

[Apache License](LICENSE)
