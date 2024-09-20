## 💡 Example Usage

This repository contains example manifests that demonstrate the usage of the Linode BlockStorage CSI Driver. These manifests create a PersistentVolumeClaim (PVC) using the `linode-block-storage-retain` storage class and then consume it in a minimal pod. 

You can find more example manifests [here](https://github.com/linode/linode-blockstorage-csi-driver/tree/main/internal/driver/examples/kubernetes).

### Creating a PersistentVolumeClaim

```sh
kubectl create -f https://raw.githubusercontent.com/linode/linode-blockstorage-csi-driver/master/internal/driver/examples/kubernetes/csi-pvc.yaml
kubectl create -f https://raw.githubusercontent.com/linode/linode-blockstorage-csi-driver/master/internal/driver/examples/kubernetes/csi-app.yaml
```

**Verify the Pod and PVC:**

```sh
kubectl get pvc/csi-example-pvc pods/csi-example-pod
kubectl describe pvc/csi-example-pvc pods/csi-example-pod
```

**Persist Data Example:**

1. **Write Data:**

    ```sh
    kubectl exec -it csi-example-pod -- /bin/sh -c "echo persistence > /data/example.txt; ls -l /data"
    ```

2. **Delete and Recreate Pod:**

    ```sh
    kubectl delete pods/csi-example-pod
    kubectl create -f https://raw.githubusercontent.com/linode/linode-blockstorage-csi-driver/master/pkg/linode-bs/examples/kubernetes/csi-app.yaml
    ```

3. **Verify Data Persistence:**

    ```sh
    sleep 30
    kubectl exec -it csi-example-pod -- /bin/sh -c "ls -l /data; cat /data/example.txt"
    ```
