---
nav_order: 11
---

# 🚀 Testing

This repository provides unit tests and three Kubernetes test suites. The Kubernetes suites can run against either a disposable Cluster API Provider Linode (CAPL) cluster or an existing Linode Kubernetes Engine (LKE) cluster.

| Suite | Command | Purpose |
| --- | --- | --- |
| Unit tests | `mise run test` | Tests driver packages in the development image. |
| Chainsaw e2e tests | `mise run e2e-test` | Exercises common CSI workflows, including encrypted volumes. |
| CSI sanity tests | `mise run csi-sanity-test` | Checks CSI RPC behavior through the driver. |
| Upstream Kubernetes storage tests | `mise run upstream-e2e-tests` | Runs the non-disruptive `External.Storage` Kubernetes conformance tests. |

## Prerequisites

Follow the [development setup](./development-setup.md) first and run `mise install`. Mise installs the pinned test tooling, including `kubectl`, `chainsaw`, `csi-sanity`, `clusterctl`, and `ctlptl`.

The Kubernetes test workflows additionally require:

- A Docker-compatible container engine available through the `docker` CLI, such as Docker Engine or Colima.
- A container registry where you can push a test image, and credentials to push to it.
- A Linode Personal Access Token with read/write access to Linodes and Volumes. Keep the token in your shell environment or a local secret manager. Do not commit it, add it to manifests, or pass it on a command line that may be retained in shell history.

Set the values used by the test setup:

```sh
export LINODE_TOKEN="your-linode-api-token"
export LINODE_REGION="us-east"
```

`LINODE_REGION` selects the region for CAPL-provisioned infrastructure. The driver determines volume topology from the nodes it serves. Each Kubernetes workflow creates a `linode` Secret in the `kube-system` namespace containing `LINODE_TOKEN`.

## Run Unit Tests

Run the unit suite from the repository root:

```sh
mise run test
```

The command builds the development image when necessary and writes coverage data to `coverage.out`.

## Prepare a Test Image

Kubernetes tests must use an image containing the driver revision under test. Build and push it to a public registry that the test cluster can pull from:

```sh
export REGISTRY_NAME="index.docker.io"
export DOCKER_USER="your-account"
export IMAGE_NAME="linode-blockstorage-csi-driver"
export IMAGE_VERSION="local-$(git rev-parse --short HEAD)"
export CSI_IMAGE_NAME="$REGISTRY_NAME/$DOCKER_USER/$IMAGE_NAME"

docker login "$REGISTRY_NAME"
mise run image-build
mise run image-push
```

Use a new `IMAGE_VERSION` after every change you want to test. The controller image uses the `IfNotPresent` pull policy, so reusing a tag can run a cached image rather than your latest local build.

For a private registry, configure image-pull credentials in the cluster before installing the driver. Do not place registry credentials in this repository.

## Option 1: Create a CAPL Test Cluster

This workflow creates a local kind management cluster, provisions a temporary Linode workload cluster through CAPL, installs the test image, and writes its kubeconfig to `test-cluster-kubeconfig.yaml`.

```sh
export MANAGEMENT_KUBECONFIG="$HOME/.kube/config"
export KUBECONFIG="$MANAGEMENT_KUBECONFIG"
export K8S_VERSION="v1.36.2"
export CONTROLPLANE_NODES="1"
export WORKER_NODES="1"

mise run mgmt-cluster
mise run capl-cluster
export KUBECONFIG="$PWD/test-cluster-kubeconfig.yaml"
```

After changing the driver, publish a new image version, regenerate the driver manifest, apply it, and wait for both workloads to use the new image:

```sh
export IMAGE_VERSION="local-$(git rev-parse --short HEAD)-$(date +%s)"

mise run image-build
mise run image-push
hack/generate-yaml.sh "$IMAGE_VERSION" "$CSI_IMAGE_NAME" > csi-manifests.yaml
kubectl --kubeconfig "$KUBECONFIG" apply -f csi-manifests.yaml
kubectl --kubeconfig "$KUBECONFIG" rollout status -n kube-system daemonset/csi-linode-node --timeout=600s
kubectl --kubeconfig "$KUBECONFIG" rollout status -n kube-system statefulset/csi-linode-controller --timeout=600s
```

`mise run cleanup-cluster` deletes CAPL-managed clusters and the local kind management cluster. It must run against the management cluster, not the workload cluster:

```sh
KUBECONFIG="$MANAGEMENT_KUBECONFIG" mise run cleanup-cluster
```

## Option 2: Use an Existing LKE Cluster

Use a dedicated non-production LKE cluster. The driver installation replaces any existing release manifests with the generated test manifest, so do not use a production cluster or one running a driver version that must remain available.

Point `KUBECONFIG` at the LKE cluster, create or update the driver Secret without exposing its value in a manifest, generate the test manifests, install them, and wait for the new image to roll out:

```sh
export KUBECONFIG="/path/to/lke-kubeconfig.yaml"

kubectl --kubeconfig "$KUBECONFIG" create secret generic linode \
  --namespace kube-system \
  --from-literal=token="$LINODE_TOKEN" \
  --from-literal=region="$LINODE_REGION" \
  --dry-run=client -o yaml | kubectl --kubeconfig "$KUBECONFIG" apply -f -

hack/generate-yaml.sh "$IMAGE_VERSION" "$CSI_IMAGE_NAME" > csi-manifests.yaml
kubectl --kubeconfig "$KUBECONFIG" apply -f csi-manifests.yaml
kubectl --kubeconfig "$KUBECONFIG" rollout status -n kube-system daemonset/csi-linode-node --timeout=600s
kubectl --kubeconfig "$KUBECONFIG" rollout status -n kube-system statefulset/csi-linode-controller --timeout=600s
```

Confirm that every node has a ready CSI node pod before running tests:

```sh
kubectl --kubeconfig "$KUBECONFIG" get pods -n kube-system -l app=csi-linode-node
kubectl --kubeconfig "$KUBECONFIG" get pods -n kube-system -l app=csi-linode-controller
```

The `cleanup-cluster` task does not remove an existing LKE cluster. Remove only the driver resources and test data that you installed after reviewing the generated manifest.

## Run Kubernetes Test Suites

Run all Chainsaw tests:

```sh
mise run e2e-test
```

The command creates a local `luks.key` and passes it to the test suite. Run a labeled subset with `E2E_SELECTOR`:

```sh
E2E_SELECTOR=luksmove mise run e2e-test
```

Run CSI sanity tests:

```sh
mise run csi-sanity-test
```

CSI sanity temporarily patches the CSI node DaemonSet and creates a `csi-socat` StatefulSet. Run it only on a dedicated test cluster and wait for the script to finish before changing the driver deployment.

Run the upstream Kubernetes storage tests:

```sh
mise run upstream-e2e-tests
```

This suite downloads the Kubernetes test binaries for `K8S_VERSION`, runs non-disruptive `External.Storage` coverage, and can run for up to two hours.

## Cleanup

After any workflow, remove test-created PersistentVolumeClaims, PersistentVolumes, and volumes that remain after a failed test. The CSI sanity task removes its temporary StatefulSet and restores the CSI node DaemonSet. For CAPL-created infrastructure, use the management-cluster cleanup command in Option 1.

For an existing LKE cluster, retain the cluster and remove only the driver manifests, Secret, and test resources that you created for this run after confirming they are not in use.

Remove the locally generated encryption key after Chainsaw tests:

```sh
rm -f luks.key
```
