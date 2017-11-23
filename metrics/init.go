package metrics

import (
	"time"

	metrics "github.com/rcrowley/go-metrics"
)

func init() {
	go CaptureCPUTime(time.Minute)
	metrics.RegisterRuntimeMemStats(metrics.DefaultRegistry)
	go metrics.CaptureRuntimeMemStats(metrics.DefaultRegistry, time.Minute)
}
