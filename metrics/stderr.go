package metrics

import (
	"time"

	log "github.com/sirupsen/logrus"
	metrics "github.com/rcrowley/go-metrics"
)

// SetupStderr will configure metric system to periodically print metrics on
// stderr.
func SetupStderr() {
	go metrics.Log(metrics.DefaultRegistry, time.Minute, log.StandardLogger())
}
