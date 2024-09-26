## 🛠️ Developer Setup

### 📦 Prerequisites

- **Go**: Ensure you have Go installed. You can download it from [here](https://golang.org/dl/).
- **Docker**: Required for building and testing Docker images. Download from [here](https://www.docker.com/get-started).
- **kubectl**: Kubernetes command-line tool. Install instructions [here](https://kubernetes.io/docs/tasks/tools/).
- **Helm**: Package manager for Kubernetes. Install instructions [here](https://helm.sh/docs/intro/install/).
- **Devbox**: For managing development environments. Install instructions [here](https://www.jetify.com/devbox/docs/installing_devbox/).

### 🚀 Setting Up the Local Development Environment

1. **Clone the Repository**

    ```sh
    git clone https://github.com/linode/linode-blockstorage-csi-driver.git
    cd linode-blockstorage-csi-driver
    ```

2. **Install Devbox**

    Follow the [Devbox installation guide](https://www.jetify.com/devbox/docs/installing_devbox/) to set up Devbox on your machine.

3. **Setup Environment Variables**

    Create a `.env` file in the root directory or export them directly in your shell:

    ```sh
    export LINODE_API_TOKEN="your-linode-api-token"
    export LINODE_REGION="your-preferred-region"
    export KUBERNETES_VERSION=v1.21.0
    export LINODE_CONTROL_PLANE_MACHINE_TYPE=g6-standard-2
    export LINODE_MACHINE_TYPE=g6-standard-2
    ```

4. **Start Devbox Environment**

    ```sh
    devbox shell
    ```

    This command initializes the development environment with all necessary dependencies.

### 🛠️ Building the Project

To build the project binaries in a container(builds are run in a docker container to allow consistent builds regardless of underlying unix/linux systems):

```sh
make docker-build
```

### 🧪 Running Unit Tests

To run the unit tests, use the Dockerfile.dev that copies the directory into the container allowing us to run make targets:

```sh
export DOCKERFILE=Dockerfile.dev
make docker-build && make test
```

### 🧪 Create a Development Cluster

To set up a development cluster for running any e2e testing/workflows, follow these steps:

1. **Setup a CAPL Management Cluster**

    ```sh
    devbox run mgmt-cluster
    ```

2. **Build and Push Test Image**

    Before building and pushing the test image, ensure you've made the necessary changes to the codebase for your testing purposes.

    ```sh
    # Build the Docker image with your changes
    make docker-build IMAGE_TAG=ghcr.io/yourusername/linode-blockstorage-csi-driver:test

    # Push the image to the container registry
    make docker-push IMAGE_TAG=ghcr.io/yourusername/linode-blockstorage-csi-driver:test
    ```

    Note: Replace `yourusername` with your actual GitHub username or organization name.

    If you need to make changes to the Dockerfile or build process:
    1. Modify the `Dockerfile` in the project root if needed.
    2. Update the `Makefile` if you need to change build arguments or processes.
    3. If you've added new dependencies, ensure they're properly included in the build.

    After pushing, verify that your image is available in the GitHub Container Registry before proceeding to create the test cluster.

3. **Create a CAPL Child Test Cluster**

    ```sh
    IMAGE_NAME=ghcr.io/yourusername/linode-blockstorage-csi-driver IMAGE_VERSION=test devbox run capl-cluster
    ```

This will create a testing cluster with the necessary components to run end-to-end testing or workflows for the Linode BlockStorage CSI Driver.

For more detailed instructions on running the actual end-to-end tests, refer to the [e2e Tests README](./testing.md).

### 🔧 Linting and Formatting

Ensure your code adheres to the project's coding standards by running:

```sh
make lint
```

### 📝 Documentation

Update and maintain documentation as you develop new features or make changes. Ensure that all new functionalities are well-documented in the `README.md` or relevant documentation files.

