package consul

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"github.com/hashicorp/consul/api"
	log "github.com/sirupsen/logrus"

	"github.com/allegro/mesos-executor"
	"github.com/allegro/mesos-executor/hook"
	"github.com/allegro/mesos-executor/mesosutils"
	"github.com/allegro/mesos-executor/runenv"
	"github.com/mesos/mesos-go/api/v1/lib"
)

const (
	// Only Marathon apps with this label will be registered in Consul
	// See: https://github.com/allegro/marathon-consul/blob/v1.1.0/apps/app.go#L10-L11
	consulNameLabelKey = "consul"
	consulTagValue     = "tag"
	proxyLabelKey      = "proxy"
	serviceHost        = "127.0.0.1"
)

// instance represents a service in consul
type instance struct {
	consulServiceName string
	consulServiceID   string
	port              uint32
	tags              []string
}

// Hook is an executor hook implementation that will register and deregister a service instance
// in Consul right after startup and just before task termination, respectively.
type Hook struct {
	config           Config
	client           *api.Client
	serviceInstances []instance
}

// Config is Consul hook configuration settable from environment
type Config struct {
	// Enabled is a flag to control whether hook should be used
	Enabled bool `default:"true" envconfig:"consul_hook_enabled"`
	// Consul ACL Token
	ConsulToken string `default:"" envconfig:"consul_token"`
	// ConsulGlobalTag is a tag added to every service registered in Consul.
	// When executor fails (e.g., OOM, host restarted) task will NOT
	// be deregistered. This should be done by remote service reconciling
	// state between mesos and consul. We are using marathon-consul.
	// It requires task to be tagged with common tag: "marathon" by default.
	// > common tag name added to every service registered in Consul,
	// > should be unique for every Marathon-cluster connected to Consul
	// https://github.com/allegro/marathon-consul/blob/1.4.2/config/config.go#L74
	ConsulGlobalTag string   `default:"marathon" envconfig:"consul_global_tag"`
	ProxyCommand    []string `default:"" envconfig:"consul_proxy_command"`
}

// HandleEvent calls appropriate hook functions that correspond to supported
// event types. Unsupported events are ignored.
func (h *Hook) HandleEvent(event hook.Event) (hook.Env, error) {
	switch event.Type {
	case hook.AfterTaskHealthyEvent:
		return nil, h.RegisterIntoConsul(event.TaskInfo)
	case hook.BeforeTerminateEvent:
		return nil, h.DeregisterFromConsul(event.TaskInfo)
	default:
		log.Debugf("Received unsupported event type %s - ignoring", event.Type)
		return nil, nil // ignore unsupported events
	}
}

// RegisterIntoConsul generates an id and sends service information to Consul Agent
func (h *Hook) RegisterIntoConsul(taskInfo mesosutils.TaskInfo) error {
	consulLabel := taskInfo.FindLabel(consulNameLabelKey)

	if consulLabel == nil {
		log.Infof("Label %q not found - not registering in Consul", consulNameLabelKey)
		return nil
	}

	serviceName := taskInfo.GetLabelValue(consulNameLabelKey)
	taskID := taskInfo.GetTaskID()
	if serviceName == "true" || serviceName == "" {
		// Sanitize taskID for use as a Consul service name. Marathon uses the following patterns for taskId:
		// https://github.com/mesosphere/marathon/blob/v1.5.1.2/src/main/scala/mesosphere/marathon/state/PathId.scala#L109-L116
		serviceName = marathonAppNameToServiceName(taskID)
		log.Warnf(
			"Warning! Invalid Consul service name provided for app! Will use default app name %s instead",
			serviceName,
		)
	}

	ports := taskInfo.GetPorts()
	globalTags := append(taskInfo.GetLabelKeysByValue(consulTagValue), h.config.ConsulGlobalTag)

	var instancesToRegister []instance
	for _, port := range ports {
		portServiceName, err := getServiceLabel(port)
		if err != nil {
			log.Debugf("Pre-registration check for port failed: %s", err.Error())
			continue
		}
		// consulServiceID is generated the same way as it is in marathon-consul - because
		// it registers the service
		// See: https://github.com/allegro/marathon-consul/blob/v1.1.0/consul/consul.go#L299-L301
		consulServiceID := fmt.Sprintf("%s_%s_%d", taskID, portServiceName, port.GetNumber())
		portTags := mesosutils.GetLabelKeysByValue(port.GetLabels().GetLabels(), consulTagValue)
		portTags = append(portTags, globalTags...)
		log.Infof("Adding service ID %q to deregister before termination", consulServiceID)
		instancesToRegister = append(instancesToRegister, instance{
			consulServiceName: portServiceName,
			consulServiceID:   consulServiceID,
			port:              port.GetNumber(),
			tags:              portTags,
		})
	}

	if len(instancesToRegister) == 0 {
		serviceID := fmt.Sprintf("%s_%s_%d", taskID, serviceName, ports[0].GetNumber())
		instancesToRegister = []instance{
			{
				consulServiceName: serviceName,
				consulServiceID:   serviceID,
				port:              ports[0].GetNumber(),
				tags:              globalTags,
			},
		}
	}

	connectConfig, err := h.getConnectConfig(taskInfo)
	if err != nil {
		return errors.Wrap(err, "Creating Consul Connect configuration failed")
	}

	agent := h.client.Agent()
	for _, serviceData := range instancesToRegister {
		serviceRegistration := api.AgentServiceRegistration{
			ID:                serviceData.consulServiceID,
			Name:              serviceData.consulServiceName,
			Tags:              serviceData.tags,
			Port:              int(serviceData.port),
			Address:           runenv.IP().String(),
			EnableTagOverride: false,
			Checks:            api.AgentServiceChecks{},
			Check:             generateHealthCheck(taskInfo.GetHealthCheck(), int(serviceData.port)),
			Connect:           connectConfig,
		}

		if err := agent.ServiceRegister(&serviceRegistration); err != nil {
			log.WithError(err).Warnf("Unable to register service ID %q in Consul agent", serviceData.consulServiceID)
			return fmt.Errorf("registration in Consul failed: %s", err.Error())
		}
		log.Debugf("Service %q registered in Consul with port %d and ID %q", serviceData.consulServiceName, serviceData.port, serviceData.consulServiceID)
		log.Infof("Adding service ID %q to deregister before termination", serviceData.consulServiceID)
		h.serviceInstances = append(h.serviceInstances, serviceData)
	}

	return nil
}

// DeregisterFromConsul will deregister service IDs from Consul that were created
// during AfterTaskStartEvent hook event.
func (h *Hook) DeregisterFromConsul(taskInfo mesosutils.TaskInfo) error {
	agent := h.client.Agent()

	var ghostInstances []instance
	for _, serviceData := range h.serviceInstances {
		if err := agent.ServiceDeregister(serviceData.consulServiceID); err != nil {
			// Consul will deregister ghost instances after some time
			log.WithError(err).Warnf("Unable to deregister service ID %s in Consul agent", serviceData.consulServiceID)
			// we still want to try deregistering if this hook gets called again
			ghostInstances = append(ghostInstances, serviceData)
		}
	}
	h.serviceInstances = ghostInstances

	return nil
}

func (h *Hook) getConnectConfig(taskInfo mesosutils.TaskInfo) (*api.AgentServiceConnect, error) {
	proxyLabel := taskInfo.FindLabel(proxyLabelKey)
	if proxyLabel == nil || proxyLabel.GetValue() == "false" {
		return nil, nil
	}
	log.Info("Starting proxy for service")
	cmd := h.config.ProxyCommand
	if len(cmd) == 0 || cmd[0] == "" {
		return nil, errors.Errorf(
			"'%s' label found, but proxy command is not set in executor configuration", proxyLabelKey)
	}
	log.Debugf("Proxy command: %v", h.config.ProxyCommand)
	executable, err := os.Executable()
	if err != nil {
		return nil, errors.Wrap(err, "Cannot obtain executable path of current process")
	}
	executorPath, err := filepath.Abs(executable)
	if err != nil {
		return nil, errors.Wrapf(err, "Cannot get absolute path of file '%s'", executorPath)
	}
	config := &api.AgentServiceConnect{
		Native: false,
		Proxy: &api.AgentServiceConnectProxy{
			Command:  cmd,
			ExecMode: api.ProxyExecModeDaemon,
			Config: map[string]interface{}{
				"executor_path": executorPath,
			},
		},
	}
	return config, nil
}

func getServiceLabel(port mesos.Port) (string, error) {
	label := mesosutils.FindLabel(port.GetLabels().GetLabels(), consulNameLabelKey)
	if label == nil {
		return "", fmt.Errorf("port %d has no label %q", port.GetNumber(), consulNameLabelKey)
	}
	return label.GetValue(), nil
}

func marathonAppNameToServiceName(name mesosutils.TaskID) string {
	var sanitizer = strings.NewReplacer("_", ".", "/", "-")
	// Remove all spaces and initial slashes, replace above characters
	var sanitizedName = sanitizer.Replace(strings.Trim(strings.TrimSpace(string(name)), "/"))
	if strings.Contains(sanitizedName, ".") {
		var parts = strings.Split(sanitizedName, ".")
		return strings.Join(parts[0:len(parts)-1], ".")
	}
	return sanitizedName
}

func generateHealthCheck(mesosCheck mesosutils.HealthCheck, port int) *api.AgentServiceCheck {
	check := api.AgentServiceCheck{}
	check.Interval = mesosCheck.Interval.String()
	check.Timeout = mesosCheck.Timeout.String()

	switch mesosCheck.Type {
	case mesosutils.HTTP:
		check.HTTP = generateURL(mesosCheck.HTTP.Path, port)
	case mesosutils.TCP:
		check.TCP = fmt.Sprintf("%s:%d", serviceHost, port)
	}
	return nil
}

func generateURL(path string, port int) string {
	var checkURL url.URL
	checkURL.Host = executor.HealthCheckAddress(uint32(port))
	checkURL.Path = path

	return checkURL.String()
}

// NewHook creates new Consul hook that is responsible for graceful Consul deregistration.
func NewHook(cfg Config) (hook.Hook, error) {
	if !cfg.Enabled {
		return hook.NoopHook{}, nil
	}
	config := api.DefaultConfig()
	config.Token = cfg.ConsulToken
	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}
	return &Hook{config: cfg, client: client}, err
}
