package metrics

import (
	"fmt"
	"net"
	"time"

	log "github.com/Sirupsen/logrus"
	graphite "github.com/cyberdelia/go-metrics-graphite"
	"github.com/kelseyhightower/envconfig"
	metrics "github.com/rcrowley/go-metrics"
)

const graphiteConfigEnvPrefix = "allegro_executor_graphite"

func init() {
	var cfg GraphiteConfig
	if err := envconfig.Process(graphiteConfigEnvPrefix, &cfg); err != nil {
		log.WithError(err).Fatal("Invalid graphite configuration")
	}
	if cfg.Host != "" {
		if err := SetupGraphite(cfg); err != nil {
			log.WithError(err).Fatal("Invalid graphite configuration")
		} else {
			log.Info("Metrics will be sent to Graphite")
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
		return fmt.Errorf("Invalid Graphite address: %s", err)
	}
	go graphite.Graphite(metrics.DefaultRegistry, time.Minute, cfg.Prefix, addr)
	return nil
}
