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

	NodeUnpublishTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_unpublish_total",
			Help: "Total number of NodeUnpublishVolume calls",
		},
		[]string{"success"},
	)

	NodeUnpublishDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_unpublish_duration_seconds",
			Help:    "Duration of NodeUnpublishVolume calls",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"success"},
	)

	NodeStageVolumeTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_stage_volume_total",
			Help: "Total number of NodeStageVolume calls",
		},
		[]string{"success"},
	)

	NodeStageVolumeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_stage_volume_duration_seconds",
			Help:    "Duration of NodeStageVolume calls",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"success"},
	)

	NodeUnstageVolumeTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_unstage_volume_total",
			Help: "Total number of NodeUnstageVolume calls",
		},
		[]string{"success"},
	)

	NodeUnstageVolumeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_unstage_volume_duration_seconds",
			Help:    "Duration of NodeUnstageVolume calls",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"success"},
	)

	NodeExpandTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "csi_node_expand_total",
			Help: "Total number of NodeExpandVolume calls",
		},
		[]string{"success"},
	)

	NodeExpandDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "csi_node_expand_duration_seconds",
			Help:    "Duration of NodeExpandVolume calls",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"success"},
	)
)

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
