# Linode Block Storage CSI Driver - AI Coding Instructions

## Architecture Overview

This is a Kubernetes CSI (Container Storage Interface) driver for Linode Block Storage volumes. The driver implements the CSI specification with three main server components:

- **ControllerServer** (`internal/driver/controllerserver.go`) - Handles volume lifecycle (create/delete/attach/detach/resize)
- **NodeServer** (`internal/driver/nodeserver.go`) - Handles node-level operations (mount/unmount/stage/unstage)
- **IdentityServer** (`internal/driver/identityserver.go`) - Provides driver identity and capabilities

The driver runs in two modes:
- **Controller mode**: Manages volume lifecycle via Linode API
- **Node mode**: Handles local volume operations on Kubernetes nodes

## CSI RPC Implementation Patterns

### Controller Service (`ControllerServer`)
Implements volume lifecycle management with required capabilities:
- `CREATE_DELETE_VOLUME`: Volume provisioning/deprovisioning via Linode API
- `PUBLISH_UNPUBLISH_VOLUME`: Volume attachment/detachment to/from nodes
- `EXPAND_VOLUME`: Online/offline volume expansion support
- `CREATE_DELETE_SNAPSHOT`: Snapshot lifecycle management

### Node Service (`NodeServer`) 
Handles node-local volume operations:
- `STAGE_UNSTAGE_VOLUME`: Block device staging to global mount point
- `GET_VOLUME_STATS`: Volume usage statistics for monitoring
- `EXPAND_VOLUME`: Filesystem expansion after controller expansion

### Identity Service (`IdentityServer`)
Required by all CSI plugins:
- `GetPluginInfo`: Returns driver name (`linodebs.csi.linode.com`) and version
- `GetPluginCapabilities`: Advertises `CONTROLLER_SERVICE` and `VOLUME_ACCESSIBILITY_CONSTRAINTS`
- `Probe`: Health check with optional readiness reporting

### RPC Interaction Rules
Per CSI spec section on RPC interactions and reference counting:
- `NodeStageVolume` MUST be called before any `NodePublishVolume` 
- All `NodeUnpublishVolume` MUST complete before `NodeUnstageVolume`
- `ControllerUnpublishVolume` called after all Node operations complete
- CO responsible for reference counting when `STAGE_UNSTAGE_VOLUME` advertised

## Key Architectural Patterns

### Dependency Injection & Interface Design
The driver uses extensive dependency injection with interfaces for testability:
- `LinodeClient` interface wraps the Linode API client (`pkg/linode-client/`)
- `DeviceUtils` interface handles device discovery (`pkg/device-manager/`)
- `SafeMounter` interface wraps mount operations (`pkg/mount-manager/`)
- `Encryption` interface handles LUKS encryption (`internal/driver/luks.go`)

### Package Structure Convention
```
pkg/           # Reusable packages with interfaces
internal/      # Driver-specific logic
mocks/         # Generated mocks for testing
tests/         # E2E, CSI sanity, and upstream tests
```

### Error Handling
Use gRPC status codes throughout CSI implementations per spec requirements:
```go
// Volume doesn't exist
return status.Error(codes.NotFound, "volume not found")

// Volume already exists but incompatible  
return status.Error(codes.AlreadyExists, "volume exists with different parameters")

// Invalid parameters
return status.Error(codes.InvalidArgument, "missing required field: volume_id")

// Resource limits exceeded
return status.Error(codes.ResourceExhausted, "max volumes per node exceeded")
```

**Status Message Guidelines**: 
- MUST contain human-readable description if not `OK`
- MAY be surfaced to end users by CO
- SHOULD provide actionable information for debugging

## Critical Development Workflows

### Building & Testing
```bash
# Build in container (required for consistent cross-platform builds)
export DOCKERFILE=Dockerfile.dev  # Use dev Dockerfile for development
make docker-build

# Run unit tests (in container)
make test

# Generate mocks after interface changes
make generate-mock

# Full CI pipeline
make ci  # vet + lint + test + build
```

### E2E Testing Workflow
The project uses a sophisticated E2E setup with CAPI (Cluster API) and CAPL (Cluster API Provider Linode):

```bash
# Create management cluster + workload cluster with CSI driver
make mgmt-and-capl-cluster

# Run E2E tests with chainsaw
make e2e-test

# Run CSI sanity tests
make csi-sanity-test

# Clean up clusters
make cleanup-cluster
```

Environment variables required:
- `LINODE_TOKEN` - Linode API token
- `LINODE_REGION` - Target region for testing

### Release Process
```bash
make release IMAGE_VERSION=v1.2.3
```
Generates release manifests and Helm chart in `release/` directory.

## Project-Specific Conventions

### Volume Naming & Labeling
- Volume names use prefix pattern: `pvc-{uuid}` for K8s PVC integration
- Optional volume label prefix via `LINODE_VOLUME_LABEL_PREFIX` (max 12 chars, regex: `^[0-9A-Za-z_-]{0,12}$`)
- Volume tags support via CSI parameters (`volumeTags` annotation)
- Volume IDs are plugin-generated and MUST be unique within plugin scope

### Volume Size Constraints (CSI CapacityRange)
- Linode enforces 10Gi minimum - driver handles transparently by provisioning larger size
- `CapacityRange.required_bytes`: Minimum size (MUST be honored even if larger than Linode minimum)
- `CapacityRange.limit_bytes`: Maximum size (MUST NOT be exceeded)
- Driver MUST validate range and return `OUT_OF_RANGE` if unsupported

### Encryption (LUKS)
Supports LUKS encryption via CSI parameters per CSI secrets requirements:
- `csi.storage.k8s.io/luks-encrypted: "true"`
- `csi.storage.k8s.io/luks-cipher`, `csi.storage.k8s.io/luks-key-size`
- Encryption key from Secret referenced in StorageClass (`secrets` field in CSI requests)
- Secrets MUST be treated as sensitive - never logged or exposed
- Encryption keys MUST be unique across all volumes
- Uses `cryptsetup` for LUKS operations (`pkg/cryptsetup-client/`)

### Topology Awareness
Driver supports zone-aware provisioning per CSI `VOLUME_ACCESSIBILITY_CONSTRAINTS`:
- Uses Linode metadata service for node region/zone detection (`pkg/linode-bs/`)
- Respects `TopologyRequirement.requisite` and `TopologyRequirement.preferred` in `CreateVolumeRequest`
- Returns `Volume.accessible_topology` in `CreateVolumeResponse`
- Node plugin MUST implement `NodeGetInfo` to report node topology via `accessible_topology` field

### Observability
- Structured logging via logr throughout (`pkg/logger/`)
- Prometheus metrics (opt-in via `ENABLE_METRICS`)
- OpenTelemetry tracing (opt-in via `OTEL_TRACING`)
- Pre-built Grafana dashboard in `observability/metrics/`

## Critical Integration Points

### Linode API Integration
- Uses `linodego` client library
- Implements retry logic and rate limiting
- Metadata service integration for node information (`pkg/linode-bs/`)

### CSI Specification Compliance
The driver implements CSI v1.x specification with strict compliance requirements:

**Idempotency**: All operations MUST be idempotent per CSI spec
- `CreateVolume`: Returns `0 OK` if volume exists with compatible parameters
- `DeleteVolume`: Returns `0 OK` if volume doesn't exist (no-op)
- `ControllerPublishVolume`: Returns `0 OK` if already published with compatible capability
- `NodeStageVolume`/`NodePublishVolume`: Handle already-staged/published scenarios

**Volume Lifecycle Management** (see CSI spec Figure 5-6):
```
CreateVolume → ControllerPublishVolume → NodeStageVolume → NodePublishVolume → PUBLISHED
```
- Uses staging/unstaging pattern for block devices (`STAGE_UNSTAGE_VOLUME` capability)
- Node operations depend on successful controller operations
- Proper cleanup order: NodeUnpublishVolume → NodeUnstageVolume → ControllerUnpublishVolume → DeleteVolume

**Error Handling**: Use standard gRPC error codes throughout:
- `codes.InvalidArgument` (3): Missing/invalid required fields
- `codes.NotFound` (5): Volume/node doesn't exist  
- `codes.AlreadyExists` (6): Volume exists but incompatible
- `codes.FailedPrecondition` (9): Volume in use, exceeds capabilities
- `codes.Aborted` (10): Operation pending for volume
- `codes.Unimplemented` (12): RPC not implemented
- `codes.ResourceExhausted` (8): Storage/attachment limits exceeded

**Volume Capabilities**: Driver supports specific access modes and volume types
- Block volumes: `VolumeCapability.BlockVolume`
- Mount volumes: `VolumeCapability.MountVolume` with filesystem options
- Access modes: `SINGLE_NODE_WRITER`, `MULTI_NODE_READER_ONLY` (see `capabilities.go`)

**Topology Awareness**: Implements `VOLUME_ACCESSIBILITY_CONSTRAINTS`
- Volumes accessible only from same region as creation
- Uses Linode metadata service for node topology detection

### Kubernetes Integration
- Uses client-go for Kubernetes API access
- Supports volume expansion, snapshots, and cloning
- Integrates with CSI external components (sidecars):
  - `external-provisioner`: Watches PVC creation, calls `CreateVolume`
  - `external-attacher`: Manages `ControllerPublishVolume`/`ControllerUnpublishVolume`
  - `external-resizer`: Handles volume expansion via `ControllerExpandVolume`
  - `external-snapshotter`: Manages snapshot lifecycle (This project does not implement snapshots yet)
  - `csi-node-driver-registrar`: Registers driver with kubelet on each node
- Driver deployed as StatefulSet (controller) + DaemonSet (node plugin)
- Uses Unix domain sockets for gRPC communication (`CSI_ENDPOINT` environment variable)

## Testing Patterns

### Mock Generation
All interfaces have generated mocks in `mocks/`. Regenerate after interface changes:
```bash
make generate-mock
```

### Test Structure
- Unit tests alongside source files (`*_test.go`)
- E2E tests use chainsaw framework in `tests/e2e/`
- CSI sanity tests validate CSI spec compliance
- Upstream Kubernetes E2E tests for storage

### Test Data Management
- LUKS keys generated via `openssl rand -out luks.key 64`
- Kubeconfig management via `test-cluster-kubeconfig.yaml`
- Test clusters use predictable naming: `csi-driver-cluster-{git-hash}`

## Development Environment

### Devbox Integration
Uses devbox for reproducible development environments:
```bash
devbox shell  # Activates dev environment with all dependencies
```

### Container-First Development
All builds and tests run in containers to ensure consistency across platforms. The `Dockerfile.dev` provides the development environment.

## Common Pitfalls

1. **Volume Attachment Limits**: Linode has per-instance attachment limits (varies by instance type)
2. **Minimum Volume Size**: Linode enforces 10Gi minimum, driver handles this transparently per CSI `CapacityRange` rules
3. **Cross-Zone Volumes**: Volumes can only be attached to nodes in the same region (topology constraint)
4. **Idempotency Requirements**: All CSI operations MUST be idempotent - check existing state before modifications
5. **Volume Capabilities**: Always validate `VolumeCapability` in requests match what volume supports
6. **gRPC Status Codes**: Use correct status codes per CSI spec - incorrect codes break CO error handling
7. **Volume Context**: `volume_context` from `CreateVolumeResponse` MUST be passed to subsequent Node operations
8. **Staging Path Requirements**: `staging_target_path` MUST be unique per volume, CO creates directory

When modifying CSI operations, always consider:
- **Idempotency**: Can operation be called multiple times safely?
- **Error Recovery**: Does error code allow CO to take appropriate action?
- **State Consistency**: Are driver and storage backend in consistent state after operation?
- **Capability Validation**: Does operation respect advertised plugin capabilities?
