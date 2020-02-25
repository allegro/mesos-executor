package consul

import (
	"fmt"
	"net/url"
	"strings"
	"time"

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

// Check that should be checked in consul before changing service state to healthy on mesos
type ServiceCheckToVerify struct {
	consulServiceName string
	check *api.AgentServiceCheck
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
	// Timeout used to wait for service health check pass in consul.
	// If service health checks are not healthy within specified timeout
	// service status will not be set to healthy.
	// Setting to zero disables health checks verification
	TimeoutForConsulHealthChecksInSeconds time.Duration `default:"0" envconfig:"consul_healtcheck_timeout"`
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
		portServiceName, err := getServiceLabel(port)
		if err != nil {
			log.Debugf("Pre-registration check for port failed: %s", err.Error())
			continue
		}
		// consulServiceID is generated the same way as it is in marathon-consul - because
		// it registers the service
		// See: https://github.com/allegro/marathon-consul/blob/v1.1.0/consul/consul.go#L299-L301
		consulServiceID := fmt.Sprintf("%s_%s_%d", taskID, portServiceName, port.GetNumber())
		marathonTaskTag := fmt.Sprintf("marathon-task:%s", taskID)
		portTags := mesosutils.GetLabelKeysByValue(port.GetLabels().GetLabels(), consulTagValue)
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
	var checksToVerifyAfterRegistration []ServiceCheckToVerify
	agent := h.client.Agent()
	for _, serviceData := range instancesToRegister {
		serviceHealthCheck := generateHealthCheck(serviceData.consulServiceID, taskInfo.GetHealthCheck(), int(serviceData.port))

		serviceRegistration := api.AgentServiceRegistration{
			ID:                serviceData.consulServiceID,
			Name:              serviceData.consulServiceName,
			Tags:              resolvePlaceholders(serviceData.tags, tagPlaceholders),
			Port:              int(serviceData.port),
			Address:           runenv.IP().String(),
			EnableTagOverride: false,
			Checks:            api.AgentServiceChecks{},
			Check:             serviceHealthCheck,
		}

		if err := agent.ServiceRegister(&serviceRegistration); err != nil {
			log.WithError(err).Warnf("Unable to register service ID %q in Consul agent", serviceData.consulServiceID)
			return fmt.Errorf("registration in Consul failed: %s", err.Error())
		}
		log.Debugf("Service %q registered in Consul with port %d and ID %q", serviceData.consulServiceName, serviceData.port, serviceData.consulServiceID)
		log.Infof("Adding service ID %q to deregister before termination", serviceData.consulServiceID)
		h.serviceInstances = append(h.serviceInstances, serviceData)
		checksToVerifyAfterRegistration = append(checksToVerifyAfterRegistration, ServiceCheckToVerify {
			consulServiceName: serviceData.consulServiceName,
			check: serviceHealthCheck,
		})
	}
	log.Infof("Checking status of registered consul health checks")
	return h.VerifyConsulChecksAfterRegistrationWithTimeout(checksToVerifyAfterRegistration)
}

// Waits for change all service health checks status to passing
// It this state does not change within defined TimeoutForConsulHealthChecksInSeconds timeout
// service will not be marked as healthy on Mesos
func (h *Hook) VerifyConsulChecksAfterRegistrationWithTimeout(checksToVerifyAfterRegistration []ServiceCheckToVerify) error {
	if h.config.TimeoutForConsulHealthChecksInSeconds == 0 {
		return nil
	}
	c := make(chan bool, 1)
	go func() {
		for len(checksToVerifyAfterRegistration) > 0 {
			checksToVerifyAfterRegistration = h.VerifyConsulChecks(checksToVerifyAfterRegistration)

		}
		c <- true
	} ()
	select {
	case <- c:
		return nil
	case <-time.After(h.config.TimeoutForConsulHealthChecksInSeconds * time.Second):
		log.Warnf("After %d seconds %d health checks still fails", h.config.TimeoutForConsulHealthChecksInSeconds, len(checksToVerifyAfterRegistration))
		return fmt.Errorf("after %d seconds %d health checks still fails", h.config.TimeoutForConsulHealthChecksInSeconds, len(checksToVerifyAfterRegistration))
	}
}

func (h *Hook) VerifyConsulChecks(checksToVerifyAfterRegistration []ServiceCheckToVerify) []ServiceCheckToVerify{
	var checksLeftToVerify []ServiceCheckToVerify
	health := h.client.Health()
	OUTER:
	for _, checkToVerify := range checksToVerifyAfterRegistration {
		serviceChecksResult,_ , err := health.Checks(checkToVerify.consulServiceName, nil)
		if  err != nil {
			log.WithError(err).Warnf("Error during checking health check for service %q", checkToVerify.consulServiceName)
			continue
		}
		for _, currentCheckResult := range serviceChecksResult {
			if currentCheckResult.CheckID == checkToVerify.check.CheckID {
				if currentCheckResult.Status == api.HealthPassing {
					continue OUTER
				}
			}
			checksLeftToVerify = append(checksLeftToVerify, checkToVerify)
		}
	}
	if len(checksLeftToVerify) > 0 {
		log.Infof("Sleep for 1 second before next verification of health checks")
		time.Sleep(time.Second)
	}
	return checksLeftToVerify
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

func generateHealthCheck(serviceId string, mesosCheck mesosutils.HealthCheck, port int) *api.AgentServiceCheck {
	check := api.AgentServiceCheck{}
	check.CheckID = "service:" + serviceId
	check.Interval = mesosCheck.Interval.String()
	check.Timeout = mesosCheck.Timeout.String()

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
