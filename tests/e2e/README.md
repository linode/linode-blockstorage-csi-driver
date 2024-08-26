## How to use these End-to-end (e2e) tests

In order to run these e2e tests, you'll need the following:
- CAPL Management Cluster
- CAPL Child Test Cluster
- Test Image 

### 0. Pre-requisites: Install devbox

Install devbox as described in the [devbox docs](https://www.jetify.com/devbox/docs/installing_devbox/) before running any of the commands below.

### 1. Setup a CAPL Management Cluster

We will be using a kind cluster and install CAPL plus various other providers.

Setup the env vars and run the following command to create a kind mgmt cluster:

```sh
# Make sure to set the following env vars
export LINODE_TOKEN="your linode api token"
export LINODE_REGION="your preferred region"
export KUBERNETES_VERSION=v1.29.1
export LINODE_CONTROL_PLANE_MACHINE_TYPE=g6-standard-2
export LINODE_MACHINE_TYPE=g6-standard-2

devbox run local-deploy
```
This will download all the necessary binaries to local bin and create a local mgmt cluster.

### 2. Build and Push Test Image

If you have a PR open, GHA will build & push to docker hub and tag it with the current branch name.

If you do not have PR open, follow the steps below:
- Build a docker image passing the `IMAGE_TAG` argument to the make target
  so a custom tag is applied. Then push the image to a public repository.

  > You can use any public repository that you have access to. The tags used below are just examples

  ```
  make docker-build IMAGE_TAG=ghcr.io/avestuk/linode-blockstorage-csi-driver:test-e2e
  make docker-push IMAGE_TAG=ghcr.io/avestuk/linode-blockstorage-csi-driver:test-e2e
  ```

### 2. Setup a CAPL Child Test Cluster

In order create a test cluster, run the following command:

```sh
TEST_IMAGE_NAME=ghcr.io/avestuk/linode-blockstorage-csi-driver TEST_IMAGE_TAG=test-e2e devbox run remote-cluster-deploy
```
> You don't need to pass TEST_IMAGE_TAG and TEST_IMAGE_NAME if you have a PR open

The above command will create a test cluster, install CSI driver using the test image, and export kubeconfig of test-cluster to the root directory

### 3. Run E2E Tests

Run the following command to run e2e tests:

```sh
devbox run e2e-test
```
This will run the chainsaw e2e tests under the `e2e/test` directory

### 4. Cleanup

Run the following command to cleanup the test cluster:

```sh
devbox run cleanup-cluster
```
*Its will destroy the CAPL test cluster and kind mgmt cluster*
