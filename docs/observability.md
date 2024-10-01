# Observability with Grafana Dashboard

This document explains how to use the `grafana-dashboard` make target to install and configure observability tools, including Prometheus and Grafana, on your Kubernetes cluster. The setup uses Helm charts to install Prometheus and Grafana, provides a Prometheus data source, and applies a Grafana dashboard configuration.

## Prerequisites

Ensure the following tools are installed on your local machine:
- **Kubernetes**: A running Kubernetes cluster.
- **kubectl**: To manage the cluster.
- **Helm**: To install and manage Helm charts for Prometheus and Grafana.

You should also have access to the Kubernetes cluster's kubeconfig file (`test-cluster-kubeconfig.yaml`), which will be used for running the make target.

## Steps to Install the Grafana Dashboard

### 1. Build and Set Up the Cluster (Optional)
If you haven’t already set up your Kubernetes cluster with the necessary CSI driver and Prometheus metrics services, you can do so by running the following command:
```bash
make mgmt-and-capl-cluster
```
This command creates a management cluster and CAPL (Cluster API for Linode) cluster, installs the Linode CSI driver, and applies the necessary configurations to expose the CSI metrics.

### 2. Run the Grafana Dashboard Setup
The make target `grafana-dashboard` will install Prometheus and Grafana in a Kubernetes namespace (`monitoring` by default) and configure a Grafana dashboard from a local JSON file. To execute this setup, run:

```bash
make grafana-dashboard
```

### 3. What Happens During the Setup?

- **Node Scheduling**: The script checks whether worker nodes exist in the cluster. If no worker nodes are found, it untaints the control-plane nodes to allow Prometheus and Grafana to be scheduled.

- **Helm Repository Setup**: Helm repositories for Prometheus and Grafana are added if not already present.

- **Prometheus Installation**: The `prometheus` Helm chart is installed (or upgraded) in the monitoring namespace. It scrapes metrics from the CSI driver and other services.

- **Grafana Installation**: The `grafana` Helm chart is installed (or upgraded) in the same namespace, with Prometheus set as the data source.

- **Dashboard ConfigMap**: The script uploads the Grafana dashboard configuration stored locally in `docs/dashboard.json` into a Kubernetes ConfigMap, ensuring the dashboard is automatically loaded into Grafana.

- **Port Forwarding**: The script port-forwards the Grafana service to `localhost:3000`, allowing you to access the Grafana dashboard on your local machine.

### 4. Accessing the Grafana Dashboard

Once the setup is complete, open your web browser and navigate to:
```
http://localhost:3000
```

Log in using the following credentials:
- Username: `admin`
- Password: `admin`

These details can be customized via the `install-monitoring-tools.sh` script if needed. 

To understand the graphs and the metrics, go through the [Metrics Documentation](metrics-documentation.md).

### 5. Stopping the Port Forwarding

To stop the Grafana port forwarding, run:
```bash
kill <PID>
```
Replace `<PID>` with the process ID provided by the script during the setup.

## Customizing the Setup

- **Namespace**: The default namespace for the observability tools is `monitoring`. You can modify this by editing the `install-monitoring-tools.sh` script and changing the `NAMESPACE` variable.

- **Grafana Dashboard Configuration**: The dashboard configuration is stored in `docs/dashboard.json`. To apply a different dashboard, replace the contents of this file before running the make target.

- **Prometheus Data Source**: The default data source is Prometheus, as defined in the Helm chart configuration. If you wish to use a different data source, modify the `helm upgrade` command in `install-monitoring-tools.sh`.

## Troubleshooting

If you encounter issues during the installation process, check the logs and status of the Prometheus and Grafana pods:
```bash
kubectl get pods -n monitoring
kubectl logs <prometheus-pod-name> -n monitoring
kubectl logs <grafana-pod-name> -n monitoring
```

This setup provides a quick and easy way to enable observability using Grafana dashboards, ensuring that you have visibility into your Kubernetes cluster and CSI driver operations.