package consul

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/hashicorp/consul/api"
	log "github.com/sirupsen/logrus"

	executor "github.com/allegro/mesos-executor"
	"github.com/allegro/mesos-executor/hook"
	"github.com/allegro/mesos-executor/mesosutils"
	"github.com/allegro/mesos-executor/runenv"
	mesos "github.com/mesos/mesos-go/api/v1/lib"
)

const (
	// Only Marathon apps with this label will be registered in Consul
	// See: https://github.com/allegro/marathon-consul/blob/v1.1.0/apps/app.go#L10-L11
	consulNameLabelKey = "consul"
	consulTagValue     = "tag"
	serviceHost        = "127.0.0.1"
	portPlaceholder    = "{port:%s}"
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
	ConsulGlobalTag string `default:"marathon" envconfig:"consul_global_tag"`
	// Status that will be set for registered service health check
	// By default we assume service health was checked initially by marathon
	// It will be set to passing.
	InitialHealthCheckStatus string `default:"passing" envconfig:"initial_health_check_status"`
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
	tagPlaceholders := getPlaceholders(ports)
	globalTags := append(taskInfo.GetLabelKeysByValue(consulTagValue), h.config.ConsulGlobalTag)

	var instancesToRegister []instance
	for _, port := range ports {
		portServiceNames, err := getServiceLabels(port)
		if err != nil {
			log.Debugf("Pre-registration check for port failed: %s", err.Error())
			continue
		}

		for _, portServiceName := range portServiceNames {
			// consulServiceID is generated the same way as it is in marathon-consul - because
			// it registers the service
			// See: https://github.com/allegro/marathon-consul/blob/v1.1.0/consul/consul.go#L299-L301
			consulServiceID := fmt.Sprintf("%s_%s_%d", taskID, portServiceName, port.GetNumber())
			marathonTaskTag := fmt.Sprintf("marathon-task:%s", taskID)
			portTags := getPortTags(port, portServiceName)
			portTags = append(portTags, globalTags...)
			portTags = append(portTags, marathonTaskTag)
			log.Infof("Adding service ID %q to deregister before termination", consulServiceID)
			instancesToRegister = append(instancesToRegister, instance{
				consulServiceName: portServiceName,
				consulServiceID:   consulServiceID,
				port:              port.GetNumber(),
				tags:              portTags,
			})
		}
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

	agent := h.client.Agent()
	for _, serviceData := range instancesToRegister {
		serviceRegistration := api.AgentServiceRegistration{
			ID:                serviceData.consulServiceID,
			Name:              serviceData.consulServiceName,
			Tags:              resolvePlaceholders(serviceData.tags, tagPlaceholders),
			Port:              int(serviceData.port),
			Address:           runenv.IP().String(),
			EnableTagOverride: false,
			Checks:            api.AgentServiceChecks{},
			Check:             h.generateHealthCheck(taskInfo.GetHealthCheck(), int(serviceData.port)),
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

func getPortTags(port mesos.Port, serviceName string) []string {
	var keys []string
	labels := port.GetLabels().GetLabels()

	for _, label := range labels {
		value := label.GetValue()
		valueAndSelector := strings.Split(value, ":")
		if len(valueAndSelector) > 1 {
			value := valueAndSelector[0]
			serviceSelector := valueAndSelector[1]

			if value == consulTagValue && serviceSelector == serviceName {
				keys = append(keys, label.GetKey())
			}
		} else if value == consulTagValue {
			keys = append(keys, label.GetKey())
		}
	}

	return keys
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

func (h *Hook) generateHealthCheck(mesosCheck mesosutils.HealthCheck, port int) *api.AgentServiceCheck {
	check := api.AgentServiceCheck{}
	check.Interval = mesosCheck.Interval.String()
	check.Timeout = mesosCheck.Timeout.String()
	check.Status = h.config.InitialHealthCheckStatus

	switch mesosCheck.Type {
	case mesosutils.HTTP:
		check.HTTP = generateURL(mesosCheck.HTTP.Path, port)
		return &check
	case mesosutils.TCP:
		check.TCP = fmt.Sprintf("%s:%d", serviceHost, port)
		return &check
	}
	return nil
}

func getPlaceholders(ports []mesos.Port) map[string]string {
	placeholders := map[string]string{}
	for _, port := range ports {
		name := port.GetName()
		if name != "" {
			placeholder := fmt.Sprintf(portPlaceholder, name)
			placeholders[placeholder] = fmt.Sprint(port.GetNumber())
		}
	}
	return placeholders
}

func resolvePlaceholders(values []string, placeholders map[string]string) []string {
	resolved := make([]string, 0, len(values))
	for _, value := range values {
		for placeholder, replacement := range placeholders {
			value = strings.Replace(value, placeholder, replacement, -1)
		}
		resolved = append(resolved, value)
	}
	return resolved
}

func getServiceLabels(port mesos.Port) ([]string, error) {
	label := mesosutils.FindLabel(port.GetLabels().GetLabels(), consulNameLabelKey)
	if label == nil {
		return nil, fmt.Errorf("port %d has no label %q", port.GetNumber(), consulNameLabelKey)
	}

	labels := strings.Split(label.GetValue(), ",")

	return labels, nil
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

func generateURL(path string, port int) string {
	var checkURL url.URL
	checkURL.Scheme = "http"
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
