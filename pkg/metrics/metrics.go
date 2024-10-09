package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	SuccessTrue  = "true"
	SuccessFalse = "false"
)

var (
	NodePublishTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_publish_total",
			Help: "Total number of NodePublishVolume calls",
		},
		[]string{"success"},
	)

	NodePublishDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_publish_duration_seconds",
			Help:    "Duration of NodePublishVolume calls",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"success"},
	)

	nodeUnpublishTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_unpublish_total",
			Help: "Total number of NodeUnpublishVolume calls",
		},
		[]string{"success"},
	)

	nodeUnpublishDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_unpublish_duration_seconds",
			Help:    "Duration of NodeUnpublishVolume calls",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"success"},
	)

	nodeStageVolumeTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_stage_volume_total",
			Help: "Total number of NodeStageVolume calls",
		},
		[]string{"success"},
	)

	nodeStageVolumeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_stage_volume_duration_seconds",
			Help:    "Duration of NodeStageVolume calls",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"success"},
	)

	nodeUnstageVolumeTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_unstage_volume_total",
			Help: "Total number of NodeUnstageVolume calls",
		},
		[]string{"success"},
	)

	nodeUnstageVolumeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_unstage_volume_duration_seconds",
			Help:    "Duration of NodeUnstageVolume calls",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"success"},
	)

	nodeExtendTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_extend_total",
			Help: "Total number of NodeExtendVolume calls",
		},
		[]string{"success"},
	)

	nodeExtendDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_extend_duration_seconds",
			Help:    "Duration of NodeExtendVolume calls",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"success"},
	)
)

func init() {
	prometheus.MustRegister(NodePublishTotal)
	prometheus.MustRegister(NodePublishDuration)
	prometheus.MustRegister(nodeUnpublishTotal)
	prometheus.MustRegister(nodeUnpublishDuration)
	prometheus.MustRegister(nodeStageVolumeTotal)
	prometheus.MustRegister(nodeStageVolumeDuration)
	prometheus.MustRegister(nodeUnstageVolumeTotal)
	prometheus.MustRegister(nodeUnstageVolumeDuration)
	prometheus.MustRegister(nodeExtendTotal)
	prometheus.MustRegister(nodeExtendDuration)
}
