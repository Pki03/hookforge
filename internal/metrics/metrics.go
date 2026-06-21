package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	EventsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hookforge_events_total",
		Help: "Total number of events processed by status",
	}, []string{"status"})

	DeliveryLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "hookforge_delivery_latency_seconds",
		Help:    "Latency of event deliveries in seconds",
		Buckets: prometheus.DefBuckets,
	})

	RetryCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "hookforge_retry_events_current",
		Help: "Current number of events in retry queue",
	})

	DeliveryAttempts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "hookforge_delivery_attempts_total",
		Help: "Total number of delivery attempts",
	})
)
