## 🚀 Deployment

### 🔧 Requirements

- **Kubernetes** v1.16+
- **LINODE_API_TOKEN**: [Personal Access Token](https://cloud.linode.com/profile/tokens) with:
  - Read/Write access to Volumes
  - Read/Write access to Linodes
  - Sufficient "Expiry" for continued use
- **LINODE_REGION**: [Linode Region](https://api.linode.com/v4/regions)

### 🔐 Secure a Linode API Access Token

Generate a Personal Access Token (PAT) using the [Linode Cloud Manager](https://cloud.linode.com/profile/tokens).

### ⚙️ Deployment Methods

There are two primary methods to deploy the CSI Driver:

1. **Using Helm (Recommended)**
2. **Using kubectl**

#### 🔧 Prerequisites for running the driver

- The deployment assumes that the Linode Cloud Controller Manager (CCM) is running.

#### 1. Using Helm

##### 🔄 Install the csi-linode Repo

```sh
helm repo add linode-csi https://linode.github.io/linode-blockstorage-csi-driver/
helm repo update linode-csi
```

##### 🚀 Deploy the CSI Driver

```sh
export LINODE_API_TOKEN="...your Linode API token..."
export REGION="your preferred region"

helm install linode-csi-driver \
  --set apiToken="${LINODE_API_TOKEN}" \
  --set region="${REGION}" \
  linode-csi/linode-blockstorage-csi-driver
```

_See [helm install](https://helm.sh/docs/helm/helm_install/) for command documentation._

##### 🧹 Uninstalling the CSI Driver

```sh
helm uninstall linode-csi-driver
```

_See [helm uninstall](https://helm.sh/docs/helm/helm_uninstall/) for command documentation._

##### ⬆️ Upgrading the CSI Driver

```sh
export LINODE_API_TOKEN="...your Linode API token..."
export REGION="your preferred region"
helm upgrade linode-csi-driver \
--install \
--set apiToken="${LINODE_API_TOKEN}" \
--set region="${REGION}" \
linode-csi/linode-blockstorage-csi-driver
```

_See [helm upgrade](https://helm.sh/docs/helm/helm_upgrade/) for command documentation._

##### ⚙️ Configurations

- Modify variables using the `--set var=value` flag or by providing a custom `values.yaml` with `-f custom-values.yaml`.
- For a comprehensive list of configurable variables, refer to [`helm-chart/csi-driver/values.yaml`](https://github.com/linode/linode-blockstorage-csi-driver/blob/main/helm-chart/csi-driver/values.yaml).

###### Controller kubeconfig (optional)

If your environment requires the controller to use a kubeconfig file explicitly, enable the controller kubeconfig by providing the following values. The Secret will be mounted as a directory and the sidecars will read the file `<mountDir>/<secretKey>`.

```yaml
controller:
  kubeconfig:
    mountDir: /etc/kubeconfig
    secretName: csi-kubeconfig
    secretKey: external-kubeconfig
```

Helm example:

```sh
helm install linode-csi-driver \
  --set apiToken="$LINODE_API_TOKEN" \
  --set region="$REGION" \
  --set controller.kubeconfig.mountDir=/etc/kubeconfig \
  --set controller.kubeconfig.secretName=csi-kubeconfig \
  --set controller.kubeconfig.secretKey=external-kubeconfig \
  linode-csi/linode-blockstorage-csi-driver
```

The Secret should contain a key named `external-kubeconfig` (or your chosen `secretKey`). For example:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: csi-kubeconfig
  namespace: kube-system
stringData:
  external-kubeconfig: |
    # contents of your kubeconfig file
```

###### Controller ServiceAccount and RBAC toggles

By default, the chart creates the controller ServiceAccount and its RBAC ClusterRoleBindings. You can disable these if you want to manage them externally. The controller ServiceAccount name defaults to `csi-controller-sa`.

```yaml
controller:
  serviceAccount:
    enabled: true   # set to false to skip creating the SA (still referenced by the StatefulSet)
    name: ""        # optionally override the ServiceAccount name (defaults to "csi-controller-sa")
  rbac:
    enabled: true   # set to false to skip creating controller ClusterRoleBindings
```

When `controller.serviceAccount.enabled=false`, ensure a ServiceAccount named `csi-controller-sa` exists in the target namespace.

###### DaemonSet ServiceAccount and RBAC toggles

By default, the chart also creates the node DaemonSet ServiceAccount and its RBAC ClusterRoleBinding. You can disable these if you manage them externally. The node ServiceAccount name defaults to `csi-node-sa`.

```yaml
daemonSet:
  serviceAccount:
    enabled: true   # set to false to skip creating the node SA (DaemonSet will omit serviceAccount when false)
    name: ""        # optionally override the ServiceAccount name (defaults to "csi-node-sa")
  rbac:
    enabled: true   # set to false to skip creating the node ClusterRoleBinding
```

When `daemonSet.serviceAccount.enabled=false`, ensure a ServiceAccount named `csi-node-sa` exists in the target namespace if you intend to set it explicitly on the DaemonSet yourself.

##### 👉 Recommendation

Use a custom `values.yaml` file to override variables to avoid template rendering errors.

#### 2. Using kubectl

##### 🔑 Create a Secret

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

Apply the secret to the cluster:

```sh
kubectl apply -f secret.yaml
```
Verify the secret was created:

```sh
kubectl get secret linode -n kube-system
```

##### 🚀 Deploy the CSI Driver

Apply the deployment manifest (changes on the main branch):

```sh
kubectl apply -f https://raw.githubusercontent.com/linode/linode-blockstorage-csi-driver/master/internal/driver/deploy/releases/linode-blockstorage-csi-driver.yaml
```

To deploy a specific release version:

```sh
kubectl apply -f https://raw.githubusercontent.com/linode/linode-blockstorage-csi-driver/master/internal/driver/deploy/releases/linode-blockstorage-csi-driver-v0.5.3.yaml
```

### 🔧 Advanced Configuration and Operational Details

1. **Storage Classes**
   - **Default Storage Class**: `linode-block-storage-retain` [Learn More](https://kubernetes.io/docs/tasks/administer-cluster/change-default-storage-class/)
     - **Reclaim Policy**: `Retain`
       - Volumes created by this CSI driver will default to using the `linode-block-storage-retain` storage class if one is not specified. Upon deletion of all PersistentVolumeClaims, the PersistentVolume and its backing Block Storage Volume will remain intact.
       - This behavior can be modified in the `csi-storageclass.yaml` section of the deployment by toggling the `storageclass.kubernetes.io/is-default-class` annotation.
   - **Alternative Storage Class**: `linode-block-storage`
     - **Reclaim Policy**: `Delete`
       - This policy will delete the volume when the PersistentVolumeClaim (PVC) or PersistentVolume (PV) is deleted.

    You can verify the storage classes with:

    ```sh
    kubectl get storageclass
    ```

2. **Linode Cloud Controller Manager (CCM)**
   - The deployment assumes that the Linode CCM is initialized and running.
   - If you intend to run this on a cluster without the Linode CCM, you must modify the init container script in the [`cm-get-linode-id.yaml` ConfigMap](https://github.com/linode/linode-blockstorage-csi-driver/blob/main/deploy/kubernetes/base/cm-get-linode-id.yaml) and remove the line containing `exit 1`.

3. **Maximum Volume Attachments**
   - The Linode Block Storage CSI Driver supports attaching more than 8 volumes for larger Linode instances. The maximum number of attachable volumes (including instance disks) scales with the instance's memory, up to a maximum of 64 attachments.

   | Instance Type | Memory/RAM | Max. Volume + Disk Attachments |
   |---------------|------------|--------------------------------|
   | g6-nanode-1   | 1GB        | 8                              |
   | g6-standard-1 | 2GB        | 8                              |
   | g6-standard-2 | 4GB        | 8                              |
   | g6-standard-4 | 8GB        | 8                              |
   | g6-standard-6 | 16GB       | 16                             |
   | g6-standard-8 | 32GB       | 32                             |
   | g6-standard-16| 64GB       | 64                             |
   | g6-standard-20| 96GB       | 64                             |
   | g6-standard-24| 128GB      | 64                             |
   | g6-standard-32| 192GB      | 64                             |

   This scaling also applies to dedicated, premium, GPU, and high-memory instance classes. The number of attached volumes is a combination of block storage volumes and instance disks (e.g., the boot disk).

   **Note:** To support this change, block storage volume attachments are no longer persisted across reboots.

4. **Offline Volume Resizing**

    - The CSI driver supports offline volume resizing. This means that the volume must be unmounted from all nodes before resizing.
    - To resize a volume, update the `spec.resources.requests.storage` field in the PersistentVolumeClaim (PVC) manifest and apply the changes.
    - The CSI driver will automatically resize the underlying Linode Block Storage Volume to match the new size specified in the PVC.
    - The volume must be unmounted from all nodes before resizing.
      It could be unmounted by deleting the pod using the volume or by using the `kubectl delete pod <pod-name>` command.

5. Driver Role Configuration

Set the `DRIVER_ROLE` environment variable to control which CSI components load:

- **controller**
  - **Prerequisite**: `LINODE_TOKEN` **must** be defined.
  - **Behavior**: Boots the CSI Controller service.
  - **API**: Actively calls the Linode API to provision, attach and manage volumes.

- **nodeserver**
  - **Prerequisite**: `LINODE_TOKEN` **not required**.
  - **Behavior**: Launches only the NodeServer endpoint.
  - **API**: Operates entirely offline—no Linode API interactions.
