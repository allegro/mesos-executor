package metrics

import (
	"time"

	metrics "github.com/rcrowley/go-metrics"
	log "github.com/sirupsen/logrus"
)

// SetupStderr will configure metric system to periodically print metrics on
// stderr.
func SetupStderr() {
	go metrics.Log(metrics.DefaultRegistry, time.Minute, log.StandardLogger())
}
