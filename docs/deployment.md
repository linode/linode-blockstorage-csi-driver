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
kubectl apply -f https://raw.githubusercontent.com/linode/linode-blockstorage-csi-driver/master/internal/driver/deploy/releases/linode-blockstorage-csi-driver.yaml
```

### Key Features and Considerations for CSI Driver Deployment

1. **Storage Classes**
   - **Default Storage Class**: `linode-block-storage-retain` [Learn More](https://kubernetes.io/docs/tasks/administer-cluster/change-default-storage-class/)
     - **Reclaim Policy**: `Retain`
       - Volumes created by this CSI driver will default to using the `linode-block-storage-retain` storage class if one is not specified. Upon deletion of all PersistentVolumeClaims, the PersistentVolume and its backing Block Storage Volume will remain intact.
       - This behavior can be modified in the `csi-storageclass.yaml` section of the deployment by toggling the `storageclass.kubernetes.io/is-default-class` annotation.
   - **Alternative Storage Class**: `linode-block-storage`
     - **Reclaim Policy**: `Delete`
       - This policy will delete the volume when the PersistentVolumeClaim (PVC) or PersistentVolume (PV) is deleted.

2. **Linode Cloud Controller Manager (CCM)**
   - The deployment assumes that the Linode CCM is initialized and running.
   - If you intend to run this on a cluster without the Linode CCM, you must modify the init container script in the [`cm-get-linode-id.yaml` ConfigMap](https://github.com/linode/linode-blockstorage-csi-driver/blob/main/deploy/kubernetes/base/cm-get-linode-id.yaml) and remove the line containing `exit 1`.

You can verify the storage classes with:

```sh
kubectl get storageclass
```