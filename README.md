# Linode Block Storage CSI Driver

[![Go Report Card](https://goreportcard.com/badge/github.com/linode/linode-blockstorage-csi-driver)](https://goreportcard.com/report/github.com/linode/linode-blockstorage-csi-driver)
[![codecov](https://codecov.io/gh/linode/linode-blockstorage-csi-driver/graph/badge.svg?token=b5HeEgMdAd)](https://codecov.io/gh/linode/linode-blockstorage-csi-driver)
[![Docker Pulls](https://img.shields.io/docker/pulls/linode/linode-blockstorage-csi-driver.svg)](https://hub.docker.com/r/linode/linode-blockstorage-csi-driver/)

## Table of Contents

- [Overview](#overview)
- [Deployment](docs/deployment.md)
  - [Requirements](docs/deployment.md#-requirements)
  - [Secure a Linode API Access Token](docs/deployment.md#-secure-a-linode-api-access-token)
  - [Deployment Methods](docs/deployment.md#️-deployment-methods)
    - [Using Helm (Recommended)](docs/deployment.md#1-using-helm)
    - [Using kubectl](docs/deployment.md#2-using-kubectl)
  - [Advanced Configuration and Operational Details](docs/deployment.md#-advanced-configuration-and-operational-details)
- [Usage Examples](docs/usage.md)
  - [Creating a PersistentVolumeClaim](docs/usage.md#creating-a-persistentvolumeclaim)
  - [Encrypted Drives using LUKS](docs/encrypted-drives.md)
  - [Adding Tags to Created Volumes](docs/volume-tags.md)
  - [Topology-Aware Provisioning](docs/topology-aware-provisioning.md)
- [Development Setup](docs/development-setup.md)
  - [Prerequisites](docs/development-setup.md#-prerequisites)
  - [Setting Up the Local Development Environment](docs/development-setup.md#-setting-up-the-local-development-environment)
  - [Building the Project](docs/development-setup.md#️-building-the-project)
  - [Running Unit Tests](docs/development-setup.md#️-running-unit-tests)
  - [Creating a Development Cluster](docs/development-setup.md#️-creating-a-development-cluster)
  - [Running E2E Tests](docs/testing.md)
  - [Contributing](docs/contributing.md)
- [License](#license)
- [Disclaimers](#-disclaimers)
- [Community](#-join-us-on-slack)

## 📚 Overview

The Container Storage Interface ([CSI](https://github.com/container-storage-interface/spec)) Driver for Linode Block Storage enables container orchestrators such as Kubernetes to manage the lifecycle of persistent storage claims.

For more information about Kubernetes CSI, refer to the [Kubernetes CSI](https://kubernetes-csi.github.io/docs/introduction.html) and [CSI Spec](https://github.com/container-storage-interface/spec/) repositories.

## ⚠️ Disclaimers

- **Version Compatibility**: Until this driver has reached v1.0.0, it may not maintain compatibility between driver versions.
- **Volume Size Constraints**:
  - Requests for Persistent Volumes with a require_size less than the Linode minimum Block Storage size will be fulfilled with a Linode Block Storage volume of the minimum size (currently 10Gi) in accordance with the CSI specification.
  - The upper-limit size constraint (`limit_bytes`) will also be honored, so the size of Linode Block Storage volumes provisioned will not exceed this parameter.
- **Volume Attachment Persistence**: Block storage volume attachments are no longer persisted across reboots to support a higher number of attachments on larger instances.
<!-- Add note about volume resizing limitations -->

_For more details, refer to the [CSI specification](https://github.com/container-storage-interface/spec/blob/v1.0.0/spec.md#createvolume)._

## 💬 Join Us on Slack

- **General Help/Discussion**: [Kubernetes Slack - #linode](https://kubernetes.slack.com/messages/CD4B15LUR)
- **Development/Debugging**: [Gopher's Slack - #linodego](https://gophers.slack.com/messages/CAG93EB2S)

## License

This project is licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for the full license text.
