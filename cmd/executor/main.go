package main

import (
	"fmt"
	"time"

	"github.com/evalphobia/logrus_sentry"
	"github.com/getsentry/raven-go"
	"github.com/kelseyhightower/envconfig"
	"github.com/mesos/mesos-go/api/v1/lib/executor/config"
	log "github.com/sirupsen/logrus"

	"github.com/allegro/mesos-executor"
	"github.com/allegro/mesos-executor/hook"
	"github.com/allegro/mesos-executor/hook/consul"
	"github.com/allegro/mesos-executor/hook/vaas"
	"github.com/allegro/mesos-executor/metrics"
	"github.com/allegro/mesos-executor/runenv"
)

// Version designates the version of application.
var Version string

// Config contains application configuration
var Config executor.Config

func init() {
	if err := envconfig.Process(executor.EnvironmentPrefix, &Config); err != nil {
		log.WithError(err).Fatal("Failed to load executor configuration")
	}

	if err := initSentry(Config); err != nil {
		log.WithError(err).Fatal("Failed to initialize Sentry")
	}

	if Config.Debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
}

func initSentry(config executor.Config) error {
	if len(config.SentryDSN) == 0 {
		return nil
	}

	environment, err := runenv.Environment()
	if err != nil {
		return fmt.Errorf("unable to determine runtime environment: %s", err)
	}

	if environment == runenv.LocalEnv {
		log.Infof("Disabling Sentry integration for the %s environment", environment)
		return nil
	}
	log.Infof("Enabling Sentry integration for the %s environment", environment)

	client, err := raven.New(config.SentryDSN)
	if err != nil {
		return fmt.Errorf("unable to setup raven client: %s", err)
	}
	client.SetRelease(Version)
	client.SetEnvironment(string(environment))

	sentryHook, err := logrus_sentry.NewWithClientSentryHook(client, []log.Level{
		log.PanicLevel,
		log.FatalLevel,
		log.ErrorLevel,
	})
	if err != nil {
		return fmt.Errorf("unable to setup sentry hook for logger: %s", err)
	}
	sentryHook.Timeout = time.Second
	log.AddHook(sentryHook)

	return nil
}

func createHooks() []hook.Hook {
	var consulConfig consul.Config
	readConfiguration(&consulConfig)
	consulHook, err := consul.NewHook(consulConfig)
	if err != nil {
		log.WithError(err).Fatalf("Error loading Consul hook %s", err)
	}

	var vaasConfig vaas.Config
	readConfiguration(&vaasConfig)
	vaasHook, err := vaas.NewHook(vaasConfig)
	if err != nil {
		log.WithError(err).Fatalf("Error loading VaaS service hook %s", err)
	}

	return []hook.Hook{vaasHook, consulHook}
}

func readConfiguration(config interface{}) {
	if err := envconfig.Process(executor.EnvironmentPrefix, config); err != nil {
		log.WithError(err).Fatal("Failed to load configuration")
	}
}

func main() {
	log.Infof("Allegro Mesos Executor (version: %s)", Version)

	cfg, err := config.FromEnv()
	if err != nil {
		log.WithError(err).Fatal("Failed to load Mesos configuration")
	}
	metrics.Init(cfg.ExecutorID)
	// TODO(janisz): Use custom type for configuration
	Config.MesosConfig.FrameworkID = cfg.FrameworkID
	Config.MesosConfig.ExecutorID = cfg.ExecutorID
	Config.MesosConfig.Directory = cfg.Directory
	Config.MesosConfig.Sandbox = cfg.Sandbox
	Config.MesosConfig.AgentEndpoint = cfg.AgentEndpoint
	Config.MesosConfig.ExecutorShutdownGracePeriod = cfg.ExecutorShutdownGracePeriod
	Config.MesosConfig.Checkpoint = cfg.Checkpoint
	Config.MesosConfig.RecoveryTimeout = cfg.RecoveryTimeout
	Config.MesosConfig.SubscriptionBackoffMax = cfg.SubscriptionBackoffMax
	err = executor.StartExecutor(Config, createHooks())
	if err != nil {
		log.WithError(err).Fatal("Executor exited with error")
	}
	log.Info("Executor exited successfully")
}
