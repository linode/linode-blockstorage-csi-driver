# Observability with Grafana Dashboard

This document explains how to use the `grafana-dashboard` make target to install and configure observability tools, including Prometheus and Grafana, on your Kubernetes cluster. The setup uses Helm charts to install Prometheus and Grafana, provides a Prometheus data source, and applies a Grafana dashboard configuration.

## Prerequisites

Ensure the following tools are installed on your local machine:
- **Kubernetes**: A running Kubernetes cluster.
- **kubectl**: To manage the cluster.
- **Helm**: To install and manage Helm charts for Prometheus and Grafana.

You should also have access to the Kubernetes cluster's kubeconfig file (`test-cluster-kubeconfig.yaml`), which will be used for running the make target.

Here’s a more detailed explanation of the steps for opting in to the metrics for the CSI driver. The commands involve first deleting the existing CSI driver and then reinstalling it with metrics enabled:

---

## Steps to Opt-In for the CSI Driver Metrics

To enable the metrics collection for the Linode CSI driver, follow the steps below. These steps involve exporting a new Helm template with metrics enabled, deleting the current CSI driver release, and applying the newly generated configuration.

### 1. Export the Helm Template for the CSI Driver with Metrics Enabled

First, you need to generate a new Helm template for the Linode CSI driver with the `enableMetrics` flag set to `true`. You will also have to specify an address that isn't in use for the metrics server to run on. This ensures that the CSI driver is configured to expose its metrics.

```bash
helm template linode-csi-driver \
  --set apiToken="${LINODE_API_TOKEN}" \
  --set region="${REGION}" \
  --set enableMetrics="${ENABLE_METRICS}" \
  --set metricsPort="${METRICS_PORT}" \
  helm-chart/csi-driver --namespace kube-system > csi.yaml
```

### 2. Delete the Existing Release of the CSI Driver

Before applying the new configuration, you need to delete the current release of the Linode CSI driver. This step is necessary because the default CSI driver installation does not have metrics enabled, and Helm doesn’t handle changes to some components gracefully without a clean reinstall.

```bash
kubectl delete -f csi.yaml --namespace kube-system
```

### 3. Apply the Newly Generated Template

Once the old CSI driver installation is deleted, you can apply the newly generated template that includes the metrics configuration.

```bash
kubectl apply -f csi.yaml
```

## Steps to Install the Grafana Dashboard

### 1. Build and Set Up the Cluster (Optional)
If you haven’t already set up your Kubernetes cluster with the necessary CSI driver and Prometheus metrics services, you can do so by running the following command:
```bash
make mgmt-and-capl-cluster
```
This command creates a management cluster and CAPL (Cluster API for Linode) cluster, installs the Linode CSI driver, and applies the necessary configurations to expose the CSI metrics.

### 2. Run the Grafana Dashboard Setup
The `grafana-dashboard` make target combines the installation of Prometheus, Grafana, and the dashboard configuration. It ensures that Prometheus is installed and connected to Grafana, and that a pre-configured dashboard is applied. To execute this setup, run:

```bash
make grafana-dashboard
```

#### What Happens During the Setup?

This target combines three separate make targets:
1. **`install-prometheus`**: Installs Prometheus using a Helm chart in the `monitoring` namespace. Prometheus is configured to scrape metrics from the CSI driver and other services.
2. **`install-grafana`**: Installs Grafana using a Helm chart in the `monitoring` namespace, with Prometheus as its data source.
3. **`setup-dashboard`**: Sets up a pre-configured Grafana dashboard by applying a ConfigMap containing the dashboard JSON (`observability/metrics/dashboard.json`).

#### Customizing the Setup

Sure! Here's a more concise, table-like version of the documentation for customizing the Grafana dashboard setup:

---

### 2. Run the Grafana Dashboard Setup

The `grafana-dashboard` target installs Prometheus, Grafana, and applies the dashboard configuration. To run the setup:

```bash
make grafana-dashboard
```

#### Customizing the Setup

You can customize various aspects of the setup by passing environment variables when running the `make grafana-dashboard` command. Use the following variables:

| **Variable**           | **Description**                          | **Example**                 |
|------------------------|------------------------------------------|-----------------------------|
| `DATA_RETENTION_PERIOD` | Sets the data retention period for Prometheus. | `DATA_RETENTION_PERIOD=30d` |
| `GRAFANA_USERNAME`      | Sets the Grafana admin username.         | `GRAFANA_USERNAME=myadmin`  |
| `GRAFANA_PASSWORD`      | Sets the Grafana admin password.         | `GRAFANA_PASSWORD=password` |
### Example

To set a retention period of 30 days, and customize the Grafana admin credentials:

```bash
DATA_RETENTION_PERIOD=30d GRAFANA_USERNAME=user GRAFANA_PASSWORD=securepass make grafana-dashboard
```

These variables customize the respective make targets: `install-prometheus` for Prometheus configuration and `install-grafana` for Grafana configuration.

---

### 3. Accessing the Grafana Dashboard

Once the setup is complete, you can access the Grafana dashboard through the configured LoadBalancer service. After the setup script runs, the external IP of the LoadBalancer is printed, and you can access Grafana by opening the following URL in your browser:

```
http://<LoadBalancer-EXTERNAL-IP>
```

Log in using the following credentials:
- Username: `admin`
- Password: `admin`

These credentials can be customized via environment variables in the `install-monitoring-tools.sh` script if needed.

### 4. Stopping the Port Forwarding (if used)

If you are using port forwarding instead of a LoadBalancer, and you wish to stop the forwarding, run:
```bash
kill <PID>
```
Replace `<PID>` with the process ID provided by the script during the setup.

If you do not have access to the script output, run:
```bash
ps -ef | grep 'kubectl port-forward' | grep -v grep
```
This will give you details about the process and also the `PID`.

## Customizing the Setup

- **Namespace**: The default namespace for the observability tools is `monitoring`. You can modify this by passing the `--namespace` flag or editing the `install-monitoring-tools.sh` script and changing the `NAMESPACE` variable.

- **Grafana Dashboard Configuration**: The default dashboard configuration is stored in `observability/metrics/dashboard.json`. To apply a different dashboard, replace the contents of this file before running the `make grafana-dashboard` target.

- **Prometheus Data Source**: The default data source is Prometheus, as defined in the Helm chart configuration. If you wish to use a different data source, modify the `helm upgrade` command in `install-monitoring-tools.sh`.

## Makefile Targets

### `install-prometheus`
Installs Prometheus in the `monitoring` namespace using a Helm chart. Prometheus scrapes metrics from the CSI driver and other services in the cluster.

```bash
make install-prometheus
```

### `install-grafana`
Installs Grafana in the `monitoring` namespace using a Helm chart. Prometheus is set as the data source for Grafana.

```bash
make install-grafana
```

### `setup-dashboard`
Sets up the pre-configured Grafana dashboard by applying a ConfigMap containing the dashboard JSON. This ConfigMap is created from the `observability/metrics/dashboard.json` file.

```bash
make setup-dashboard
```

### `grafana-dashboard`
This is a combined target that installs Prometheus, Grafana, and configures the Grafana dashboard. It runs the `install-prometheus`, `install-grafana`, and `setup-dashboard` targets sequentially.

```bash
make grafana-dashboard
```

## Troubleshooting

If you encounter issues during the installation process, check the logs and status of the Prometheus and Grafana pods:
```bash
kubectl get pods -n monitoring
kubectl logs <prometheus-pod-name> -n monitoring
kubectl logs <grafana-pod-name> -n monitoring
```

This setup provides a quick and easy way to enable observability using Grafana dashboards, ensuring that you have visibility into your Kubernetes cluster and CSI driver operations.

---

This updated documentation reflects the newly structured make targets for easier installation and management of Prometheus, Grafana, and the dashboard configuration. Let me know if you'd like further adjustments!