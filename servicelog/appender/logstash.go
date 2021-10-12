package appender

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/hashicorp/consul/api"
	jsoniter "github.com/json-iterator/go"
	"github.com/kelseyhightower/envconfig"
	"github.com/rcrowley/go-metrics"
	log "github.com/sirupsen/logrus"

	"github.com/allegro/mesos-executor/servicelog"
	"github.com/allegro/mesos-executor/xio"
	"github.com/allegro/mesos-executor/xnet"
)

const (
	logstashVersion      = 1
	logstashConfigPrefix = "allegro_executor_servicelog_logstash"
)

var json = jsoniter.ConfigFastest

type logstashConfig struct {
	Protocol                 string `default:"tcp"`
	Address                  string
	DiscoveryRefreshInterval time.Duration `default:"1s" split_words:"true"`
	DiscoveryServiceName     string        `split_words:"true"`

	RateLimit int `split_words:"true"`
	SizeLimit int `split_words:"true"`

	TCPKeepAlive time.Duration `default:"5s" envconfig:"tcp_keep_alive"`
	TCPTimeout   time.Duration `default:"2s" envconfig:"tcp_timeout"`
}

type logstashEntry map[string]interface{}

type logstash struct {
	writer io.Writer

	droppedBecauseOfRate    metrics.Counter
	droppedBecauseOfSize    metrics.Counter
	droppedBecauseOfTimeout metrics.Counter
	writeTimer              metrics.Timer
}

func (l *logstash) Append(entries <-chan servicelog.Entry) {
	for entry := range entries {
		if err := l.sendEntry(entry); err != nil {
			log.WithError(err).Warn("Error appending logs.")
		}
	}
}

func (l *logstash) formatEntry(entry servicelog.Entry) logstashEntry {
	formattedEntry := logstashEntry{}
	formattedEntry["@timestamp"] = entry["time"]
	formattedEntry["@version"] = logstashVersion
	formattedEntry["message"] = entry["msg"]

	for key, value := range entry {
		if key == "msg" || key == "time" {
			continue
		}
		formattedEntry[key] = value
	}

	return formattedEntry
}

func (l *logstash) sendEntry(entry servicelog.Entry) error {
	formattedEntry := l.formatEntry(entry)
	bytes, err := l.marshal(formattedEntry)
	if err != nil {
		return fmt.Errorf("unable to marshal log entry: %s", err)
	}
	log.WithField("entry", string(bytes)).Debug("Sending log entry to Logstash")
	l.writeTimer.Time(func() { _, err = l.writer.Write(bytes) })
	if err != nil {
		if err == xio.ErrSizeLimitExceeded {
			l.droppedBecauseOfSize.Inc(1)
			log.Infof("message dropped because of size: %s", string(bytes))
			return nil // returning this error will spam stdout with errors
		}
		if err == xio.ErrRateLimitExceeded {
			l.droppedBecauseOfRate.Inc(1)
			return nil // returning this error will spam stdout with errors
		}
		if e, ok := err.(net.Error); ok && e.Timeout() {
			l.droppedBecauseOfTimeout.Inc(1)
			return nil // returning this error will spam stdout with errors
		}
		return fmt.Errorf("unable to write to Logstash server: %s", err)
	}
	return nil
}

func (l *logstash) marshal(entry logstashEntry) ([]byte, error) {
	bytes, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}
	// Logstash reads logs line by line so we need to add a newline after each
	// generated JSON entry
	bytes = append(bytes, '\n')
	return bytes, nil
}

// NewLogstash creates new appender that will send log entries to Logstash using
// passed writer.
func NewLogstash(writer io.Writer, options ...func(*logstash) error) (Appender, error) {
	l := &logstash{
		writer:                  writer,
		droppedBecauseOfRate:    metrics.GetOrRegisterCounter("servicelog.logstash.dropped.RateExceeded", metrics.DefaultRegistry),
		droppedBecauseOfSize:    metrics.GetOrRegisterCounter("servicelog.logstash.dropped.SizeExceeded", metrics.DefaultRegistry),
		droppedBecauseOfTimeout: metrics.GetOrRegisterCounter("servicelog.logstash.dropped.Timeout", metrics.DefaultRegistry),
		writeTimer:              metrics.GetOrRegisterTimer("servicelog.logstash.WriteTimer", metrics.DefaultRegistry),
	}
	for _, option := range options {
		if err := option(l); err != nil {
			return nil, fmt.Errorf("invalid config option: %s", err)
		}
	}
	return l, nil
}

// NewConsulLogstashWriter creates a new writer that will write data to instances
// provided by local Consul agent. It will use round robin algorithm to spread
// logs evenly to every Logstash instance. For TCP connections customised dialer
// can be optionally passed to have more control over how the connections are made.
func NewConsulLogstashWriter(protocol, serviceName string, refreshInterval time.Duration, dialer *net.Dialer) (io.Writer, error) {
	consulClient, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("unable to create Consul client: %s", err)
	}
	discoveryClient := xnet.NewConsulDiscoveryServiceClient(consulClient)
	instanceProvider := xnet.DiscoveryServiceInstanceProvider(serviceName, refreshInterval, discoveryClient)
	var sender xnet.Sender
	if protocol == "udp" {
		sender = &xnet.UDPSender{}
	} else {
		if dialer != nil {
			dialer = &net.Dialer{}
		}
		tcpSender := &xnet.TCPSender{
			Dialer: *dialer,
		}
		sender = tcpSender
	}
	return xnet.RoundRobinWriter(instanceProvider, sender), nil
}

// LogstashAppenderFromEnv creates the appender from the environment variables.
func LogstashAppenderFromEnv() (Appender, error) {
	config := &logstashConfig{}
	err := envconfig.Process(logstashConfigPrefix, config)
	if err != nil {
		return nil, fmt.Errorf("unable to get config from env: %s", err)
	}

	log.Info("Initializing Logstash appender with following configuration:")
	log.Infof("Protocol                 = %s", config.Protocol)
	log.Infof("Address                  = %s", config.Address)
	log.Infof("DiscoveryRefreshInterval = %s", config.DiscoveryRefreshInterval)
	log.Infof("DiscoveryServiceName     = %s", config.DiscoveryServiceName)
	log.Infof("RateLimit                = %d", config.RateLimit)
	log.Infof("SizeLimit                = %d", config.SizeLimit)
	log.Infof("TCPKeepAlive             = %s", config.TCPKeepAlive)
	log.Infof("TCPTimeout               = %s", config.TCPTimeout)

	var baseWriter io.Writer
	if len(config.DiscoveryServiceName) > 0 {
		dialer := &net.Dialer{
			KeepAlive: config.TCPKeepAlive,
			Timeout:   config.TCPTimeout,
		}
		baseWriter, err = NewConsulLogstashWriter(config.Protocol,
			config.DiscoveryServiceName, config.DiscoveryRefreshInterval, dialer)
	} else {
		baseWriter, err = net.Dial(config.Protocol, config.Address)
	}
	if err != nil {
		return nil, fmt.Errorf("invalid logstash connection data: %s", err)
	}
	var options []func(*logstash) error
	if config.RateLimit > 0 {
		options = append(options, LogstashRateLimit(config.RateLimit))
	}
	if config.SizeLimit > 0 {
		options = append(options, LogstashSizeLimit(config.SizeLimit))
	}
	return NewLogstash(baseWriter, options...)
}

// LogstashRateLimit adds rate limiting to logs sending. Logs send in higher rate
// (log lines per seconds) will be discarded.
func LogstashRateLimit(limit int) func(*logstash) error {
	return func(l *logstash) error {
		l.writer = xio.DecorateWriter(l.writer, xio.RateLimit(limit))
		return nil
	}
}

// LogstashSizeLimit adds size limiting to logs sending. Logs that exceeds passed
// size (in bytes) will be discarded.
func LogstashSizeLimit(size int) func(*logstash) error {
	return func(l *logstash) error {
		l.writer = xio.DecorateWriter(l.writer, xio.SizeLimit(size))
		return nil
	}
}
