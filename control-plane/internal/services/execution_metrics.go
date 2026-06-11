package services

import (
	"strings"
	"time"

	"github.com/hanzoai/agents/control-plane/pkg/types"

	metric "github.com/luxfi/metric"
)

var (
	queueDepthGauge = metric.NewGauge(metric.GaugeOpts{
		Name: "hanzo_agents_gateway_queue_depth",
		Help: "Number of workflow steps currently queued or in-flight for execution.",
	})

	workerInflightGauge = metric.NewGaugeVec(metric.GaugeOpts{ //nolint:unused // Reserved for future use
		Name: "hanzo_agents_worker_inflight",
		Help: "Number of active worker executions grouped by agent node.",
	}, []string{"agent"})

	stepDurationHistogram = metric.NewHistogramVec(metric.HistogramOpts{ //nolint:unused // Reserved for future use
		Name:    "hanzo_agents_step_duration_seconds",
		Help:    "Duration of workflow step executions split by terminal status.",
		Buckets: metric.DefBuckets,
	}, []string{"status"})

	stepRetriesCounter = metric.NewCounterVec(metric.CounterOpts{ //nolint:unused // Reserved for future use
		Name: "hanzo_agents_step_retries_total",
		Help: "Total number of workflow step retry attempts grouped by agent node.",
	}, []string{"agent"})

	waiterInflightGauge = metric.NewGauge(metric.GaugeOpts{
		Name: "hanzo_agents_waiters_inflight",
		Help: "Number of synchronous waiter channels currently registered.",
	})

	backpressureCounter = metric.NewCounterVec(metric.CounterOpts{
		Name: "hanzo_agents_gateway_backpressure_total",
		Help: "Count of backpressure events emitted by the execution gateway grouped by reason.",
	}, []string{"reason"})
)

func recordQueueDepth(depth int64) {
	if depth < 0 {
		depth = 0
	}
	queueDepthGauge.Set(float64(depth))
}

func recordWaiterCount(count int) {
	if count < 0 {
		count = 0
	}
	waiterInflightGauge.Set(float64(count))
}

//nolint:unused // Reserved for future use
func recordWorkerAcquire(agent string) {
	workerInflightGauge.WithLabelValues(normalizeAgentLabel(agent)).Inc()
}

//nolint:unused // Reserved for future use
func recordWorkerRelease(agent string) {
	workerInflightGauge.WithLabelValues(normalizeAgentLabel(agent)).Dec()
}

//nolint:unused // Reserved for future use
func observeStepDuration(status string, duration time.Duration) {
	normalized := types.NormalizeExecutionStatus(status)
	stepDurationHistogram.WithLabelValues(normalized).Observe(duration.Seconds())
}

//nolint:unused // Reserved for future use
func incrementStepRetry(agent string) {
	stepRetriesCounter.WithLabelValues(normalizeAgentLabel(agent)).Inc()
}

func incrementBackpressure(reason string) {
	if reason == "" {
		reason = "unknown"
	}
	backpressureCounter.WithLabelValues(strings.ToLower(reason)).Inc()
}

// RecordGatewayBackpressure increments the counter for external callers (e.g. HTTP handlers).
func RecordGatewayBackpressure(reason string) {
	incrementBackpressure(reason)
}

func normalizeAgentLabel(agent string) string {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		return "unknown"
	}
	return agent
}
