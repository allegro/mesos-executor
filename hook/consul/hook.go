package consul

import (
	"fmt"
	"net/url"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/hashicorp/consul/api"
	mesos "github.com/mesos/mesos-go/api/v1/lib"

	"github.com/allegro/mesos-executor/hook"
	"github.com/allegro/mesos-executor/mesosutils"
	"github.com/allegro/mesos-executor/runenv"
)

const (
	// Only Marathon apps with this label will be registered in Consul
	// See: https://github.com/allegro/marathon-consul/blob/v1.1.0/apps/app.go#L10-L11
	consulNameLabelKey = "consul"
	consulTagValue     = "tag"
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
	client           *api.Client
	serviceInstances []instance
}

// Config is Consul hook configuration settable from environment
type Config struct {
	// Consul ACL Token
	ConsulToken string `default:"" envconfig:"consul_token"`
}

// HandleEvent calls appropriate hook functions that correspond to supported
// event types. Unsupported events are ignored.
func (h *Hook) HandleEvent(event hook.Event) error {
	switch event.Type {
	case hook.AfterTaskHealthyEvent:
		return h.RegisterIntoConsul(event.TaskInfo)
	case hook.BeforeTerminateEvent:
		return h.DeregisterFromConsul(event.TaskInfo)
	default:
		log.Debugf("Received unsupported event type %s - ignoring", event.Type)
		return nil // ignore unsupported events
	}
}

// RegisterIntoConsul generates an id and sends service information to Consul Agent
func (h *Hook) RegisterIntoConsul(taskInfo mesos.TaskInfo) error {
	task := mesosutils.TaskInfo{TaskInfo: taskInfo}
	consulLabel := task.FindLabel(consulNameLabelKey)

	if consulLabel == nil {
		log.Infof("Label %q not found - not registering in Consul", consulNameLabelKey)
		return nil
	}

	serviceName := task.GetLabelValue(consulNameLabelKey)
	taskID := taskInfo.GetTaskID()
	if serviceName == "true" || serviceName == "" {
		// Sanitize taskID for use as a Consul service name. Marathon uses the following patterns for taskId:
		// https://github.com/mesosphere/marathon/blob/v1.5.1.2/src/main/scala/mesosphere/marathon/state/PathId.scala#L109-L116
		serviceName = marathonAppNameToServiceName(taskID.Value)
		log.Warnf(
			"Warning! Invalid Consul service name provided for app! Will use default app name %s instead",
			serviceName,
		)
	}

	ports := task.GetPorts()
	globalTags := task.GetLabelKeysByValue(consulTagValue)

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
		consulServiceID := fmt.Sprintf("%s_%s_%d", taskID.GetValue(), portServiceName, port.GetNumber())
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
		serviceID := fmt.Sprintf("%s_%s_%d", taskID.GetValue(), serviceName, ports[0].GetNumber())
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
			Tags:              serviceData.tags,
			Port:              int(serviceData.port),
			Address:           runenv.IP().String(),
			EnableTagOverride: false,
			Checks:            api.AgentServiceChecks{},
			Check:             generateHealthCheck(taskInfo.HealthCheck, int(serviceData.port)),
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
func (h *Hook) DeregisterFromConsul(taskInfo mesos.TaskInfo) error {
	agent := h.client.Agent()

	ghostInstances := []instance{}
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

func getServiceLabel(port mesos.Port) (string, error) {
	label := mesosutils.FindLabel(port.GetLabels().GetLabels(), consulNameLabelKey)
	if label == nil {
		return "", fmt.Errorf("port %d has no label %q", port.GetNumber(), consulNameLabelKey)
	}
	return label.GetValue(), nil
}

func marathonAppNameToServiceName(name string) string {
	var sanitizer = strings.NewReplacer("_", ".", "/", "-")
	// Remove all spaces and initial slashes, replace above characters
	var sanitizedName = sanitizer.Replace(strings.Trim(strings.TrimSpace(name), "/"))
	if strings.Contains(sanitizedName, ".") {
		var parts = strings.Split(sanitizedName, ".")
		return strings.Join(parts[0:len(parts)-1], ".")
	}
	return sanitizedName
}

func generateHealthCheck(mesosCheck *mesos.HealthCheck, port int) *api.AgentServiceCheck {
	check := api.AgentServiceCheck{}
	check.Interval = mesosutils.Duration(mesosCheck.GetIntervalSeconds()).String()
	check.Timeout = mesosutils.Duration(mesosCheck.GetTimeoutSeconds()).String()

	if mesosCheck.GetHTTP() != nil {
		check.HTTP = generateURL(mesosCheck.HTTP, port)

		return &check
	} else if mesosCheck.GetTCP() != nil {
		check.TCP = fmt.Sprintf("%s:%d", serviceHost, port)

		return &check
	}

	return nil
}

func generateURL(info *mesos.HealthCheck_HTTPCheckInfo, port int) string {
	const defaultHTTPScheme = "http"

	var checkURL url.URL
	checkURL.Host = fmt.Sprintf("%s:%d", serviceHost, port)
	checkURL.Path = info.GetPath()
	if info.GetScheme() != "" {
		checkURL.Scheme = info.GetScheme()
	} else {
		checkURL.Scheme = defaultHTTPScheme
	}
	return checkURL.String()
}

// NewHook creates new Consul hook that is responsible for graceful Consul deregistration.
func NewHook(cfg Config) (hook.Hook, error) {
	config := api.DefaultConfig()
	config.Token = cfg.ConsulToken
	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}
	return &Hook{client: client}, err
}
