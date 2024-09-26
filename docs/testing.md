## 🚀 How to Run End-to-End (e2e) Tests

In order to run these e2e tests, you'll need the following:
- CAPL Management Cluster
- CAPL Child Test Cluster
- Test Image 

### 📋 Pre-requisites: Setup Development Environment

Follow the steps outlined in the [development setup](./development-setup.md) to setup your development environment.

### 🏗️ Setup a CAPL Management Cluster

We will be using a kind cluster and install CAPL plus various other providers.

Setup the env vars and run the following command to create a kind mgmt cluster:

```sh
# Make sure to set the following env vars
export LINODE_TOKEN="your linode api token"
export LINODE_REGION="your preferred region"
export KUBERNETES_VERSION=v1.29.1
export LINODE_CONTROL_PLANE_MACHINE_TYPE=g6-standard-2
export LINODE_MACHINE_TYPE=g6-standard-2

devbox run mgmt-cluster
```
This will download all the necessary binaries to local bin and create a local mgmt cluster.

### 📦 Build and Push Test Image

If you have a PR open, GHA will build & push to docker hub and tag it with the current branch name.

If you do not have PR open, follow the steps below:
- Build a docker image passing the `IMAGE_TAG` argument to the make target
  so a custom tag is applied. Then push the image to a public repository.

  > You can use any public repository that you have access to. The tags used below are just examples

  ```
  make docker-build IMAGE_TAG=ghcr.io/avestuk/linode-blockstorage-csi-driver:test-e2e
  make docker-push IMAGE_TAG=ghcr.io/avestuk/linode-blockstorage-csi-driver:test-e2e
  ```

### 🔄 Setup a CAPL Child Test Cluster

In order create a test cluster, run the following command:

```sh
IMAGE_NAME=ghcr.io/avestuk/linode-blockstorage-csi-driver IMAGE_VERSION=test-e2e devbox run capl-cluster
```
> You don't need to pass IMAGE_NAME and IMAGE_VERSION if you have a PR open

The above command will create a test cluster, install CSI driver using the test image, and export kubeconfig of test-cluster to the root directory

### 🧪 Run E2E Tests

Run the following command to run all e2e tests:

```sh
devbox run e2e-test
```
This will run the chainsaw e2e tests under the `e2e/test` directory

We also label our e2e tests. The labels can be found in the `chainsaw-test.yaml` file under `metadata` in each of the individual chainsaw test directories.
This always users to select and run specific tests.
For example:
If you would like to only run the test that creates a luks volume and shuffles it between the CP and worker nodes, you could run
```sh
export E2E_SELECTOR=luksmove
devbox run e2e-test
```

### 🧹 Cleanup

Run the following command to cleanup the test cluster:

```sh
devbox run cleanup-cluster
```
*Its will destroy the CAPL test cluster and kind mgmt cluster*
