set shell := ["bash", "-cu"]

os := `uname -s | tr '[:upper:]' '[:lower:]'`
arch := `uname -m`
arch_short := if arch == "x86_64" { "amd64" } else if arch == "aarch64" { "arm64" } else { arch }
platform := env_var_or_default("PLATFORM", "linux/amd64")
dev_platform := env_var_or_default("DEV_PLATFORM", "linux/" + arch_short)
registry_name := env_var_or_default("REGISTRY_NAME", "index.docker.io")
docker_user := env_var_or_default("DOCKER_USER", "linode")
image_name := env_var_or_default("IMAGE_NAME", "linode-blockstorage-csi-driver")
rev := `git branch --show-current 2>/dev/null || echo "dev"`
dev_tag_extension := env_var_or_default("DEV_TAG_EXTENSION", "")
image_version := env_var_or_default("IMAGE_VERSION", if dev_tag_extension == "" { rev } else { rev + "-" + dev_tag_extension })
image_tag := env_var_or_default("IMAGE_TAG", registry_name + "/" + docker_user + "/" + image_name + ":" + image_version)
dev_image_tag := env_var_or_default("DEV_IMAGE_TAG", image_tag + "-dev")
csi_image_name := env_var_or_default("CSI_IMAGE_NAME", docker_user + "/" + image_name)
go_mod_cache_volume := "linode-blockstorage-csi-driver-go-mod-cache"
go_build_cache_volume := "linode-blockstorage-csi-driver-go-build-cache"
release_dir := env_var_or_default("RELEASE_DIR", "release")
dockerfile := env_var_or_default("DOCKERFILE", "Dockerfile")
e2e_selector := env_var_or_default("E2E_SELECTOR", "all")
linode_firewall_enabled := env_var_or_default("LINODE_FIREWALL_ENABLED", "true")
cluster_name := env_var_or_default("CLUSTER_NAME", "bs-csi-" + `git rev-parse --short HEAD`)
k8s_version := env_var_or_default("K8S_VERSION", "v1.36.2")
capi_version := env_var_or_default("CAPI_VERSION", "v1.13.3")
capl_version := env_var_or_default("CAPL_VERSION", "v0.10.7")
controlplane_nodes := env_var_or_default("CONTROLPLANE_NODES", "1")
worker_nodes := env_var_or_default("WORKER_NODES", "1")
grafana_port := env_var_or_default("GRAFANA_PORT", "3000")
grafana_username := env_var_or_default("GRAFANA_USERNAME", "admin")
grafana_password := env_var_or_default("GRAFANA_PASSWORD", "admin")
data_retention_period := env_var_or_default("DATA_RETENTION_PERIOD", "15d")
kubeconfig := env_var_or_default("KUBECONFIG", "test-cluster-kubeconfig.yaml")
docs_image := "jekyll/jekyll:pages"
docs_container := "linode-blockstorage-csi-driver-docs"
docs_port := "4000"
docs_livereload_port := "35729"

# Format Go source files.
fmt:
    GOFLAGS=-mod=readonly go fmt ./...

# Run Go vet after formatting source files.
vet: fmt dev-image-build
    #!/usr/bin/env bash
    set -u
    if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
        mise exec go -- env GOFLAGS=-mod=readonly go vet ./...
    else
        docker run \
            --rm \
            -w /workdir \
            -v {{ justfile_directory() }}:/workdir \
            --platform={{ dev_platform }} \
            --mount type=volume,source={{ go_mod_cache_volume }},target=/go/pkg/mod \
            --mount type=volume,source={{ go_build_cache_volume }},target=/root/.cache/go-build \
            -it \
            {{ dev_image_tag }} \
            mise exec go -- env GOFLAGS=-mod=readonly go vet ./...
    fi

# Run golangci-lint after vetting source files.
lint: vet
    #!/usr/bin/env bash
    set -u
    if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
        mise exec golangci-lint -- env GOFLAGS=-mod=readonly golangci-lint run -v -c .golangci.yml
    else
        docker run \
            --rm \
            -w /workdir \
            -v {{ justfile_directory() }}:/workdir \
            --platform={{ dev_platform }} \
            --mount type=volume,source={{ go_mod_cache_volume }},target=/go/pkg/mod \
            --mount type=volume,source={{ go_build_cache_volume }},target=/root/.cache/go-build \
            -it \
            {{ dev_image_tag }} \
            mise exec golangci-lint -- env GOFLAGS=-mod=readonly golangci-lint run -v -c .golangci.yml
    fi

# Apply golangci-lint suggested fixes after vetting source files.
lint-fix: vet
    #!/usr/bin/env bash
    set -u
    if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
        mise exec golangci-lint -- env GOFLAGS=-mod=readonly golangci-lint run -v -c .golangci.yml --fix
    else
        docker run \
            --rm \
            -w /workdir \
            -v {{ justfile_directory() }}:/workdir \
            --platform={{ dev_platform }} \
            --mount type=volume,source={{ go_mod_cache_volume }},target=/go/pkg/mod \
            --mount type=volume,source={{ go_build_cache_volume }},target=/root/.cache/go-build \
            -it \
            {{ dev_image_tag }} \
            mise exec golangci-lint -- env GOFLAGS=-mod=readonly golangci-lint run -v -c .golangci.yml --fix
    fi

# Run vulnerability checks.
vulncheck: dev-image-build
    #!/usr/bin/env bash
    set -u
    if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
        mise exec "go:golang.org/x/vuln/cmd/govulncheck" -- ./hack/vulncheck.sh
    else
        docker run \
            --rm \
            -w /workdir \
            -v {{ justfile_directory() }}:/workdir \
            --platform={{ dev_platform }} \
            --mount type=volume,source={{ go_mod_cache_volume }},target=/go/pkg/mod \
            --mount type=volume,source={{ go_build_cache_volume }},target=/root/.cache/go-build \
            -it \
            {{ dev_image_tag }} \
            mise exec "go:golang.org/x/vuln/cmd/govulncheck" -- ./hack/vulncheck.sh
    fi

# Build the nilaway-enabled golangci-lint binary.
build-nilaway: dev-image-build
    #!/usr/bin/env bash
    set -u
    if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
        mise exec golangci-lint -- golangci-lint custom
    else
        docker run \
            --rm \
            -w /workdir \
            -v {{ justfile_directory() }}:/workdir \
            --platform={{ dev_platform }} \
            --mount type=volume,source={{ go_mod_cache_volume }},target=/go/pkg/mod \
            --mount type=volume,source={{ go_build_cache_volume }},target=/root/.cache/go-build \
            -it \
            {{ dev_image_tag }} \
            mise exec golangci-lint -- golangci-lint custom
    fi

# Run nilaway checks.
nilcheck: build-nilaway
    #!/usr/bin/env bash
    set -u
    if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
        ./bin/golangci-lint-nilaway run -c .golangci-nilaway.yml
    else
        docker run \
            --rm \
            -w /workdir \
            -v {{ justfile_directory() }}:/workdir \
            --platform={{ dev_platform }} \
            --mount type=volume,source={{ go_mod_cache_volume }},target=/go/pkg/mod \
            --mount type=volume,source={{ go_build_cache_volume }},target=/root/.cache/go-build \
            -it \
            {{ dev_image_tag }} \
            ./bin/golangci-lint-nilaway run -c .golangci-nilaway.yml
    fi

# Verify Go module dependencies.
verify:
    GOFLAGS=-mod=readonly go mod verify

# Remove local build and release artifacts.
clean:
    GOOS=linux go clean -i -r -x ./...
    rm -rf _output
    rm -rf {{ release_dir }}
    rm -rf ./linode-blockstorage-csi-driver

# Build the CSI driver binary.
build:
    CGO_ENABLED=1 go build -o linode-blockstorage-csi-driver -a -ldflags '-X main.vendorVersion={{ image_version }}' ./main.go

# Serve the documentation site locally with live reload.
serve-docs:
    @echo "Serving the docs on http://localhost:{{ docs_port }}, press Ctrl-C to stop"
    docker run --rm --interactive --tty --name {{ docs_container }} --publish {{ docs_port }}:4000 --publish {{ docs_livereload_port }}:35729 --volume "{{ justfile_directory() }}:/srv/jekyll" {{ docs_image }} jekyll serve --host 0.0.0.0 --livereload --force-polling

# Build the documentation site the way GitHub Pages does.
build-docs:
    docker run --rm --volume "{{ justfile_directory() }}:/srv/jekyll" {{ docs_image }} jekyll build

# Build the driver container image.
docker-build:
    DOCKER_BUILDKIT=1 docker build --platform={{ platform }} --progress=plain -t {{ image_tag }} --build-arg REV={{ image_version }} -f ./{{ dockerfile }} .

# Build the development image used by local checks.
dev-image-build:
    if [[ -z "${GITHUB_ACTIONS:-}" ]]; then DOCKER_BUILDKIT=1 docker build --platform={{ dev_platform }} --progress=plain -t {{ dev_image_tag }} -f ./Dockerfile.dev .; fi

# Push the driver container image.
docker-push:
    docker push {{ image_tag }}

# Build and push the driver container image.
docker-setup: docker-build docker-push

# Create management and CAPL clusters with the driver installed.
mgmt-and-capl-cluster: docker-setup mgmt-cluster capl-cluster

# Create a CAPL child cluster with the driver installed.
capl-cluster: generate-capl-cluster-manifests create-capl-cluster generate-csi-driver-manifests install-csi

# Generate CAPL cluster manifests.
generate-capl-cluster-manifests:
    LINODE_FIREWALL_ENABLED={{ linode_firewall_enabled }} clusterctl generate cluster {{ cluster_name }} --kubernetes-version {{ k8s_version }} --infrastructure linode-linode:{{ capl_version }} --control-plane-machine-count {{ controlplane_nodes }} --worker-machine-count {{ worker_nodes }} | yq 'select(.metadata.name != "{{ cluster_name }}-csi-driver-linode")' > capl-cluster-manifests.yaml

# Create a CAPL child cluster and wait for it to be ready.
create-capl-cluster:
    kubectl apply -f capl-cluster-manifests.yaml
    kubectl wait --for=condition=ControlPlaneInitialized cluster/{{ cluster_name }} --timeout=600s || (kubectl get cluster -o yaml; kubectl get linodecluster -o yaml; kubectl get linodemachines -o yaml; kubectl logs -n capl-system deployments/capl-controller-manager --tail=100)
    kubectl wait --for=condition=NodeHealthy=true machines -l cluster.x-k8s.io/cluster-name={{ cluster_name }} --timeout=900s
    clusterctl get kubeconfig {{ cluster_name }} > test-cluster-kubeconfig.yaml
    KUBECONFIG={{ kubeconfig }} kubectl wait --for=condition=Ready nodes --all --timeout=600s
    cat tests/e2e/setup/linode-secret.yaml | envsubst | KUBECONFIG={{ kubeconfig }} kubectl apply -f -

# Generate CSI driver manifests.
generate-csi-driver-manifests:
    hack/generate-yaml.sh {{ image_version }} {{ csi_image_name }} > csi-manifests.yaml

# Install the CSI driver in the CAPL cluster.
install-csi:
    KUBECONFIG={{ kubeconfig }} kubectl apply -f csi-manifests.yaml
    KUBECONFIG={{ kubeconfig }} kubectl rollout status -n kube-system daemonset/csi-linode-node --timeout=600s
    KUBECONFIG={{ kubeconfig }} kubectl rollout status -n kube-system statefulset/csi-linode-controller --timeout=600s

# Create a management cluster.
mgmt-cluster:
    ctlptl apply -f tests/e2e/setup/ctlptl-config.yaml
    clusterctl init --wait-providers --wait-provider-timeout 600 --addon helm --core cluster-api:{{ capi_version }} --bootstrap kubeadm:{{ capi_version }} --control-plane kubeadm:{{ capi_version }} --infrastructure linode-linode:{{ capl_version }}

# Remove management and CAPL clusters.
cleanup-cluster:
    -kubectl delete cluster --all
    -kubectl delete linodefirewalls --all
    -kubectl delete lvpc --all
    -kind delete cluster -n capl
    rm -f luks.key

# Regenerate Go mocks.
generate-mock:
    mockgen -source=pkg/mount-manager/safe_mounter.go -destination=mocks/mock_safe-mounter.go -package=mocks
    mockgen -source=pkg/device-manager/device.go -destination=mocks/mock_device.go -package=mocks
    mockgen -source=pkg/filesystem/filesystem.go -destination=mocks/mock_filesystem.go -package=mocks
    mockgen -source=pkg/linode-client/linode_client.go -destination=mocks/mock_linodeclient.go -package=mocks
    mockgen -source=pkg/cryptsetup-client/cryptsetup_client.go -destination=mocks/mock_cryptsetupclient.go -package=mocks
    mockgen -source=internal/driver/metadata.go -destination=mocks/mock_metadata.go -package=mocks
    mockgen -source=pkg/hwinfo/hwinfo.go -destination=mocks/mock_hwinfo.go -package=mocks

# Run unit tests in the driver image.
test: dev-image-build
    #!/usr/bin/env bash
    set -u
    if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
        mise exec go -- env GOFLAGS=-mod=readonly go test ./... -cover ${TEST_ARGS:-} -coverprofile ./coverage.out
    else
        docker run \
            --rm \
            -w /workdir \
            -v {{ justfile_directory() }}:/workdir \
            --platform={{ dev_platform }} \
            --mount type=volume,source={{ go_mod_cache_volume }},target=/go/pkg/mod \
            --mount type=volume,source={{ go_build_cache_volume }},target=/root/.cache/go-build \
            --privileged \
            -it \
            {{ dev_image_tag }} \
            mise exec go -- env GOFLAGS=-mod=readonly go test ./... -cover ${TEST_ARGS:-} -coverprofile ./coverage.out
    fi

# Run end-to-end tests.
e2e-test:
    openssl rand -out luks.key 64
    KUBECONFIG={{ kubeconfig }} LUKS_KEY=$(base64 luks.key | tr -d '\n') chainsaw test ./tests/e2e --parallel 2 --selector {{ e2e_selector }}

# Run CSI sanity tests.
csi-sanity-test:
    KUBECONFIG={{ kubeconfig }} ./tests/csi-sanity/run-tests.sh

# Run upstream Kubernetes end-to-end tests.
upstream-e2e-tests:
    OS={{ os }} ARCH={{ arch_short }} K8S_VERSION={{ k8s_version }} KUBECONFIG={{ kubeconfig }} ./tests/upstream-e2e/run-tests.sh

# Run all CI checks.
ci: vet lint vulncheck test build

# Create release artifacts.
release:
    mkdir -p {{ release_dir }}
    ./hack/release-yaml.sh {{ image_version }}
    cp ./internal/driver/deploy/releases/linode-blockstorage-csi-driver-{{ image_version }}.yaml ./{{ release_dir }}
    sed -e 's/appVersion: "latest"/appVersion: "{{ image_version }}"/g' ./helm-chart/csi-driver/Chart.yaml
    tar -czvf ./{{ release_dir }}/helm-chart-{{ image_version }}.tgz -C ./helm-chart/csi-driver .

# Install Prometheus, Grafana, and the dashboard.
grafana-dashboard: install-prometheus install-grafana setup-dashboard

# Install Prometheus for monitoring.
install-prometheus:
    KUBECONFIG={{ kubeconfig }} DATA_RETENTION_PERIOD={{ data_retention_period }} ./hack/install-prometheus.sh --timeout=600s

# Install Grafana for monitoring.
install-grafana:
    KUBECONFIG={{ kubeconfig }} GRAFANA_PORT={{ grafana_port }} GRAFANA_USERNAME={{ grafana_username }} GRAFANA_PASSWORD={{ grafana_password }} ./hack/install-grafana.sh --timeout=600s

# Configure the Grafana dashboard.
setup-dashboard:
    KUBECONFIG={{ kubeconfig }} ./hack/setup-dashboard.sh --namespace=monitoring --dashboard-file=observability/metrics/dashboard.json

# Install tracing infrastructure.
setup-tracing:
    KUBECONFIG={{ kubeconfig }} ./hack/setup-tracing.sh
