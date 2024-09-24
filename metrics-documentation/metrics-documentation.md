## Grafana Dashboard Documentation: **CSI Driver Metrics**

### 1. **Introduction**
This Grafana dashboard provides an in-depth view of the CSI Driver operations for Linode Block Storage, with real-time data on volume creation, deletion, publication, and expansion. It also tracks persistent volume claims and potential runtime errors. The data is sourced from Prometheus, making it ideal for monitoring and diagnosing issues with CSI Driver operations.

### 2. **Dashboard Structure**
The dashboard is divided into several panels. Each panel focuses on a different aspect of CSI Driver operations, including Create/Delete/Publish Volume requests, runtime operation errors, and Persistent Volume (PV) and Persistent Volume Claim (PVC) events.

---

### 3. **Key Metrics and Visualizations with Graphs**

---

#### **Controller Create Volume**

- **Create Volume Requests**  
   - **Description**: Displays the total number of volume creation requests made to the CSI Driver.  
   - **Query**: `csi_sidecar_operations_seconds_count{method_name="/csi.v1.Controller/CreateVolume"}`
   - **Graph**:  
   ![Create Volume Request](example-images/create-volume-request.jpg)
   - **Explanation**: This graph shows the rate of volume creation requests over time. Spikes indicate increased provisioning activity.

- **Total Time Taken to Create Volume**  
   - **Description**: Displays the cumulative time taken to create volumes.  
   - **Query**: `csi_sidecar_operations_seconds_sum{method_name="/csi.v1.Controller/CreateVolume"}`
   - **Graph**:  
   ![Total Time to Create Volume](example-images/tt-create-volume-request.jpg)
   - **Explanation**: Tracks the total amount of time spent creating volumes, useful for identifying delays in provisioning.

---

#### **Controller Delete Volume**

- **Delete Volume Requests**  
   - **Description**: Shows the number of requests to delete volumes through the CSI Driver.  
   - **Query**: `csi_sidecar_operations_seconds_count{method_name="/csi.v1.Controller/DeleteVolume"}`
   - **Graph**:  
   ![Delete Volume Request](example-images/delete-volume-request.jpg)
   - **Explanation**: This graph tracks how often volumes are deleted. A consistent increase means regular cleanup of resources.

- **Total Time Taken to Delete Volume**  
   - **Description**: Tracks the time spent deleting volumes.  
   - **Query**: `csi_sidecar_operations_seconds_sum{method_name="/csi.v1.Controller/DeleteVolume"}`
   - **Graph**:  
   ![Total Time to Delete Volume](example-images/tt-delete-volume-request.jpg)
   - **Explanation**: Shows the time taken to delete volumes, highlighting the efficiency of resource cleanup operations.

---

#### **Controller Expand Volume**

- **Expand Volume Requests**  
   - **Description**: Monitors requests to expand volumes.  
   - **Query**: `csi_sidecar_operations_seconds_count{method_name="/csi.v1.Controller/ControllerExpandVolume"}`
   - **Graph**:  
   ![Expand Volume Request](example-images/expand-volume-request.jpg)
   - **Explanation**: This graph tracks how frequently volume expansion operations occur.

- **Total Time Taken to Expand Volume**  
   - **Description**: Displays the cumulative time taken to expand volumes.  
   - **Query**: `csi_sidecar_operations_seconds_sum{method_name="/csi.v1.Controller/ControllerExpandVolume"}`
   - **Graph**:  
   ![Total Time to Expand Volume](example-images/tt-expand-volume-request.jpg)
   - **Explanation**: Tracks the total time taken to expand volumes.

---

#### **Controller Publish Volume**

- **Publish Volume Requests**  
   - **Description**: The number of requests made to attach or publish volumes to nodes.  
   - **Query**: `csi_sidecar_operations_seconds_count{method_name="/csi.v1.Controller/ControllerPublishVolume"}`
   - **Graph**:  
   ![Publish Volume Request](example-images/publish-volume-request.jpg)
   - **Explanation**: This graph tracks how often volumes are published (attached) to nodes, indicating mounting operations.

- **Total Time Taken to Publish Volume**  
   - **Description**: Displays the cumulative time taken to publish (attach) volumes to nodes.  
   - **Query**: `csi_sidecar_operations_seconds_sum{method_name="/csi.v1.Controller/ControllerPublishVolume"}`
   - **Graph**:  
   ![Total Time to Publish Volume](example-images/tt-publish-volume-request.jpg)
   - **Explanation**: Tracks the total time spent publishing volumes to nodes.

---

#### **Controller Unpublish Volume**

- **Unpublish Volume Requests**  
    - **Description**: Tracks the number of requests to unpublish volumes.  
    - **Query**: `csi_sidecar_operations_seconds_count{method_name="/csi.v1.Controller/ControllerUnpublishVolume"}`
    - **Graph**:  
    ![Unpublish Volume Requests](example-images/unpublish-volume-request.jpg)
    - **Explanation**: This graph shows how frequently volumes are unpublished (detached) from nodes.

- **Total Time Taken to Unpublish Volume**  
    - **Description**: Displays the cumulative time taken to unpublish (detach) volumes from nodes.  
    - **Query**: `csi_sidecar_operations_seconds_sum{method_name="/csi.v1.Controller/ControllerUnpublishVolume"}`
    - **Graph**:  
    ![Total Time to Unpublish Volume](example-images/tt-unpublish-volume-request.jpg)
    - **Explanation**: Tracks the total time spent unpublishing volumes from nodes.

---

### 4. **Additional Metrics**

---

#### **Persistent Volumes (PV)**

- **Description**: Displays the total number of PV-related events that the CSI controller processed.  
- **Query**: `workqueue_adds_total{name="volumes"}`
- **Graph**:  
![Persistent Volumes](example-images/pv.jpg)
- **Explanation**: This graph shows how many PV requests were made, indicating the provisioning of new storage resources.

---

#### **Volume Claims (PVC)**

- **Description**: Tracks the number of PVC-related events that the controller reconciles.  
- **Query**: `workqueue_adds_total{name="claims"}`
- **Graph**:  
![PVC Events](example-images/pvc.jpg)
- **Explanation**: This graph tracks PVC-related events, providing insights into the frequency of new claims or bindings.

---

#### **Runtime Operation Errors**

- **Description**: Visualizes errors encountered by the CSI Driver during operations.  
- **Query**: `kubelet_runtime_operations_errors_total`
- **Graph**:  
![Runtime Operation Errors](example-images/runtime-error.jpg)
- **Explanation**: A rise in runtime errors indicates potential issues within the Kubernetes nodes or the CSI components.

---

#### **CSI Sidecar Operations Seconds Sum**

- **Description**: Shows the cumulative time taken for operations handled by CSI sidecars (attacher, provisioner, etc.).  
- **Query**: `csi_sidecar_operations_seconds_sum`
- **Graph**:  
![Sidecar Operations Seconds Sum](example-images/sidecar-operations-time-sum.jpg)
- **Explanation**: This graph tracks the total time consumed by all CSI operations, helping identify potential bottlenecks.