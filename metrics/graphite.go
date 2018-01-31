package metrics

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/cyberdelia/go-metrics-graphite"
	"github.com/kelseyhightower/envconfig"
	"github.com/rcrowley/go-metrics"
	log "github.com/sirupsen/logrus"

	"github.com/allegro/mesos-executor/runenv"
)

const graphiteConfigEnvPrefix = "allegro_executor_graphite"

var metricsID string

// Init processes the environment in search of Graphite configuration and sets up a connection
func Init(id string) {
	var cfg GraphiteConfig
	metricsID = id
	if err := envconfig.Process(graphiteConfigEnvPrefix, &cfg); err != nil {
		log.WithError(err).Fatal("Invalid graphite configuration")
	}
	if cfg.Host != "" {
		if err := SetupGraphite(cfg); err != nil {
			log.WithError(err).Fatal("Invalid graphite configuration")
		} else {
			log.Infof("Metrics will be sent to Graphite with UUID: %s", normalizeValue(metricsID))
		}
	} else {
		log.Info("No metric storage specified - using stderr to periodically print metrics")
		SetupStderr()
	}
}

// GraphiteConfig holds basic Graphite configuration.
type GraphiteConfig struct {
	Host   string
	Port   int    `default:"2003"`
	Prefix string `default:"allegro.executor"`
}

// SetupGraphite will configure metric system to periodically send metrics to
// Graphite.
func SetupGraphite(cfg GraphiteConfig) error {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port))
	if err != nil {
		return fmt.Errorf("invalid Graphite address: %s", err)
	}
	go graphite.Graphite(metrics.DefaultRegistry, time.Minute, buildUniquePrefix(cfg.Prefix), addr)
	return nil
}

func buildUniquePrefix(basePrefix string) string {
	hostname, err := runenv.Hostname()
	if err != nil {
		log.Fatalf("Unable to get hostname for metrics key: %s", err)
	}
	return fmt.Sprintf("%s.%s.%s", basePrefix, normalizeValue(hostname), normalizeValue(metricsID))
}

func normalizeValue(value string) string {
	return strings.Replace(value, ".", "_", -1)
}
