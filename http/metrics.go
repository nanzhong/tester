package http

import "github.com/prometheus/client_golang/prometheus"

const (
	// RunDurationMetricName is the name of the metric for test and benchmark run
	// durations.
	RunDurationMetricName = "run_duration_s"

	// RunLastMetricName is the name of the metric for the test and benchmark last
	// run timestamp.
	RunLastMetricName = "run_last_timestamp"
)

// RunDurationMetric is the the metric for test and benchmark run durations.
var RunDurationMetric = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: "tester",
		Subsystem: "tb",
		Name:      RunDurationMetricName,
		Help:      "Amount of time tests or benchmarks take.",
		Buckets: []float64{
			// TODO need to figure out more appropriate bucketing
			0.001, 0.01,
			0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0,
			1.25, 1.5, 1.75, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5,
			6, 7, 8, 9, 10, 15, 20, 25, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120,
			180, 240, 300, 360, 420, 480, 540, 600, 660, 720, 780, 840, 900,
		},
	},
	[]string{"name", "state"},
)

// RunLastMetric is the the metric for test and benchmark last run timestamps.
var RunLastMetric = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: "testr",
		Subsystem: "tb",
		Name:      RunLastMetricName,
		Help:      "Timestamp of the last run for a test or benchmark.",
	},
	[]string{"name", "state"},
)

func init() {
	prometheus.MustRegister(RunDurationMetric)
	prometheus.MustRegister(RunLastMetric)
}
