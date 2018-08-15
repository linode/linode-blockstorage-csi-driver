# csi-linode [![Build Status](https://travis-ci.org/displague/csi-linode.svg?branch=master)](https://travis-ci.org/displague/csi-linode)

A Container Storage Interface ([CSI](https://github.com/container-storage-interface/spec)) Driver for Linode Block Storage. The CSI plugin allows you to use Linode Block Storage with your preferred Container Orchestrator.

The Linode CSI plugin is mostly tested on Kubernetes. Theoretically it
should also work on other Container Orchestrator's, such as Mesos or
Cloud Foundry. Feel free to test it on other CO's and give us a feedback.

## Installing to Kubernetes

**Requirements:**

* Kubernetes v1.10 minimum
* `--allow-privileged` flag must be set to true for both the API server and the kubelet
* (if you use Docker) the Docker daemon of the cluster nodes must allow shared mounts

### 1. Create a secret with your Linode API Access Token:

Replace the placeholder string starting with `deadbeef..` with your own secret and
save it as `secret.yml`: 

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: linode
  namespace: kube-system
stringData:
  token: "deadbeefab1e1ead__REPLACE_ME____deadbeefab1e1ead"
  region: "us-east"
```

and create the secret using kubectl:

```sh
$ kubectl create -f ./secret.yml
secret "linode" created
```

You should now see the `linode` secret in the `kube-system` namespace along with other secrets

```sh
$ kubectl -n kube-system get secrets
NAME                  TYPE                                  DATA      AGE
default-token-jskxx   kubernetes.io/service-account-token   3         18h
linode          Opaque                                1         18h
```

#### 2. Deploy the CSI plugin and sidecars:

Before you continue, be sure to checkout to a [tagged
release](https://github.com/displague/csi-linode/releases). For
example, to use the version `v0.0.1` you can execute the following command:

```sh
kubectl apply -f https://raw.githubusercontent.com/displague/csi-linode/master/deploy/kubernetes/releases/csi-linode-v0.0.1.yaml
```

A new storage class will be created with the name `linode-block-storage` which is
responsible for dynamic provisioning. This is set to **"default"** for dynamic
provisioning. If you're using multiple storage classes you might want to remove
the annotation from the `csi-storageclass.yaml` and re-deploy it. This is
based on the [recommended mechanism](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/storage/container-storage-interface.md#recommended-mechanism-for-deploying-csi-drivers-on-kubernetes) of deploying CSI drivers on Kubernetes

*Note that the deployment proposal to Kubernetes is still a work in progress and not all of the written
features are implemented. When in doubt, open an issue or ask #sig-storage in [Kubernetes Slack](http://slack.k8s.io)*

#### 3. Test and verify

Create a PersistentVolumeClaim. This makes sure a volume is created and provisioned on your behalf:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: csi-pvc
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 5Gi
  storageClassName: linode-block-storage
```

After that create a Pod that refers to this volume. When the Pod is created, the volume will be attached, formatted and mounted to the specified Container

```yaml
kind: Pod
apiVersion: v1
metadata:
  name: my-csi-app
spec:
  containers:
    - name: my-frontend
      image: busybox
      volumeMounts:
      - mountPath: "/data"
        name: my-linode-volume
      command: [ "sleep", "1000000" ]
  volumes:
    - name: my-linode-volume
      persistentVolumeClaim:
        claimName: csi-pvc
```

Check if the pod is running successfully:

```sh
kubectl describe pods/my-csi-app
```

Write inside the app container:

```sh
$ kubectl exec -ti my-csi-app /bin/sh
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
