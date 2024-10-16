package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Constants representing success or failure states as strings for the metrics labels.
const (
	Completed = "true"  // Represents successful operation
	Failed    = "false" // Represents failed operation
)

// Metrics definitions for different CSI driver operations

// NodePublishTotal counts the total number of NodePublishVolume calls.
// It uses a label "functionStatus" to differentiate between successful and failed calls.
var (
	NodePublishTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_publish_total",
			Help: "Total number of NodePublishVolume calls"},
		[]string{"functionStatus"},
	)

	// NodePublishDuration tracks the duration of NodePublishVolume calls.
	// It also uses a "functionStatus" label to capture whether the call succeeded or failed.
	NodePublishDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_publish_duration_seconds",
			Help:    "Duration of NodePublishVolume calls",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"functionStatus"},
	)

	// NodeUnpublishTotal counts the total number of NodeUnpublishVolume calls.
	NodeUnpublishTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_unpublish_total",
			Help: "Total number of NodeUnpublishVolume calls",
		},
		[]string{"functionStatus"},
	)

	// NodeUnpublishDuration tracks the duration of NodeUnpublishVolume calls.
	NodeUnpublishDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_unpublish_duration_seconds",
			Help:    "Duration of NodeUnpublishVolume calls",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"functionStatus"},
	)

	// NodeStageVolumeTotal counts the total number of NodeStageVolume calls.
	NodeStageVolumeTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_stage_volume_total",
			Help: "Total number of NodeStageVolume calls",
		},
		[]string{"functionStatus"},
	)

	// NodeStageVolumeDuration tracks the duration of NodeStageVolume calls.
	NodeStageVolumeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_stage_volume_duration_seconds",
			Help:    "Duration of NodeStageVolume calls",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"functionStatus"},
	)

	// NodeUnstageVolumeTotal counts the total number of NodeUnstageVolume calls.
	NodeUnstageVolumeTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_unstage_volume_total",
			Help: "Total number of NodeUnstageVolume calls",
		},
		[]string{"functionStatus"},
	)

	// NodeUnstageVolumeDuration tracks the duration of NodeUnstageVolume calls.
	NodeUnstageVolumeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_unstage_volume_duration_seconds",
			Help:    "Duration of NodeUnstageVolume calls",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"functionStatus"},
	)

	// NodeExpandTotal counts the total number of NodeExpandVolume calls.
	NodeExpandTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_expand_total",
			Help: "Total number of NodeExpandVolume calls",
		},
		[]string{"functionStatus"},
	)

	// NodeExpandDuration tracks the duration of NodeExpandVolume calls.
	NodeExpandDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_expand_duration_seconds",
			Help:    "Duration of NodeExpandVolume calls",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"functionStatus"},
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
func RecordMetrics(total *prometheus.CounterVec, duration *prometheus.HistogramVec, functionStatus string, start time.Time) {
	total.WithLabelValues(functionStatus).Inc()                                   // Increment the total metric for the operation
	duration.WithLabelValues(functionStatus).Observe(time.Since(start).Seconds()) // Record the duration of the operation
}
