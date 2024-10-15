package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"time"
)

// Constants representing success or failure states as strings for the metrics labels.
const (
	SuccessTrue  = "true"  // Represents successful operation
	SuccessFalse = "false" // Represents failed operation
)

// Metrics definitions for different CSI driver operations

// NodePublishTotal counts the total number of NodePublishVolume calls.
// It uses a label "success" to differentiate between successful and failed calls.
var (
	NodePublishTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_publish_total",                  // Metric name for total publish calls
			Help: "Total number of NodePublishVolume calls", // Description of the metric
		},
		[]string{"success"}, // Label for differentiating between success/failure
	)

	// NodePublishDuration tracks the duration of NodePublishVolume calls.
	// It also uses a "success" label to capture whether the call succeeded or failed.
	NodePublishDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_publish_duration_seconds",   // Metric name for call duration
			Help:    "Duration of NodePublishVolume calls", // Description of the metric
			Buckets: prometheus.DefBuckets,                 // Default bucket intervals for histograms
		},
		[]string{"success"}, // Label for differentiating between success/failure
	)

	// NodeUnpublishTotal counts the total number of NodeUnpublishVolume calls.
	NodeUnpublishTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_unpublish_total",                  // Metric name for total unpublish calls
			Help: "Total number of NodeUnpublishVolume calls", // Description of the metric
		},
		[]string{"success"}, // Label for differentiating between success/failure
	)

	// NodeUnpublishDuration tracks the duration of NodeUnpublishVolume calls.
	NodeUnpublishDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_unpublish_duration_seconds",   // Metric name for call duration
			Help:    "Duration of NodeUnpublishVolume calls", // Description of the metric
			Buckets: prometheus.DefBuckets,                   // Default bucket intervals for histograms
		},
		[]string{"success"}, // Label for differentiating between success/failure
	)

	// NodeStageVolumeTotal counts the total number of NodeStageVolume calls.
	NodeStageVolumeTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_stage_volume_total",           // Metric name for total stage calls
			Help: "Total number of NodeStageVolume calls", // Description of the metric
		},
		[]string{"success"}, // Label for differentiating between success/failure
	)

	// NodeStageVolumeDuration tracks the duration of NodeStageVolume calls.
	NodeStageVolumeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_stage_volume_duration_seconds", // Metric name for call duration
			Help:    "Duration of NodeStageVolume calls",      // Description of the metric
			Buckets: prometheus.DefBuckets,                    // Default bucket intervals for histograms
		},
		[]string{"success"}, // Label for differentiating between success/failure
	)

	// NodeUnstageVolumeTotal counts the total number of NodeUnstageVolume calls.
	NodeUnstageVolumeTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_unstage_volume_total",           // Metric name for total unstage calls
			Help: "Total number of NodeUnstageVolume calls", // Description of the metric
		},
		[]string{"success"}, // Label for differentiating between success/failure
	)

	// NodeUnstageVolumeDuration tracks the duration of NodeUnstageVolume calls.
	NodeUnstageVolumeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_unstage_volume_duration_seconds", // Metric name for call duration
			Help:    "Duration of NodeUnstageVolume calls",      // Description of the metric
			Buckets: prometheus.DefBuckets,                      // Default bucket intervals for histograms
		},
		[]string{"success"}, // Label for differentiating between success/failure
	)

	// NodeExpandTotal counts the total number of NodeExpandVolume calls.
	NodeExpandTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_expand_total",                  // Metric name for total expand calls
			Help: "Total number of NodeExpandVolume calls", // Description of the metric
		},
		[]string{"success"}, // Label for differentiating between success/failure
	)

	// NodeExpandDuration tracks the duration of NodeExpandVolume calls.
	NodeExpandDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_expand_duration_seconds",   // Metric name for call duration
			Help:    "Duration of NodeExpandVolume calls", // Description of the metric
			Buckets: prometheus.DefBuckets,                // Default bucket intervals for histograms
		},
		[]string{"success"}, // Label for differentiating between success/failure
	)
)

// The init function registers all the defined Prometheus metrics.
func init() {
	prometheus.MustRegister(NodePublishTotal)
	prometheus.MustRegister(NodePublishDuration)
	prometheus.MustRegister(NodeUnpublishTotal)
	prometheus.MustRegister(NodeUnpublishDuration)
	prometheus.MustRegister(NodeStageVolumeTotal)
	prometheus.MustRegister(NodeStageVolumeDuration)
	prometheus.MustRegister(NodeUnstageVolumeTotal)
	prometheus.MustRegister(NodeUnstageVolumeDuration)
	prometheus.MustRegister(NodeExpandTotal)
	prometheus.MustRegister(NodeExpandDuration)
}

// RecordMetrics function is a helper to encapsulate metrics storage across function calls.
// It increments the total counter and observes the duration of the operation.
func RecordMetrics(total *prometheus.CounterVec, duration *prometheus.HistogramVec, success string, start time.Time) {
	total.WithLabelValues(success).Inc()                                   // Increment the total metric for the operation
	duration.WithLabelValues(success).Observe(time.Since(start).Seconds()) // Record the duration of the operation
}
