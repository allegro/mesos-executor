package vaas

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/allegro/mesos-executor/hook"
	"github.com/allegro/mesos-executor/mesosutils"
	"github.com/allegro/mesos-executor/runenv"
)

const vaasBackendIDKey = "vaas-backend-id"
const vaasDirectorLabelKey = "director"
const vaasAsyncLabelKey = "vaas-queue"

// vaasInitialWeight is an environment variable used to override initial weight.
const vaasInitialWeight = "VAAS_INITIAL_WEIGHT"

// canaryLabelKey is label for canary instances in mesos
const canaryLabelKey = "canary"

// Hook manages lifecycle of Varnish backend related to executed service
// instance.
type Hook struct {
	backendID    *int
	client       Client
	asyncTimeout time.Duration
}

// Config is Varnish configuration settable from environment
type Config struct {
	// Varnish as a Service API url
	VaasAPIHost string `default:"" envconfig:"vaas_host"`
	// Varnish as a Service username
	VaasAPIUsername string `default:"" envconfig:"vaas_username"`
	// Varnish as a Service access token
	VaasAPIKey string `default:"" envconfig:"vaas_token"`
	// VaasAsyncTimeout is a timeout for async registration in VaaS
	VaasAsyncTimeout time.Duration `default:"90s" envconfig:"vaas_async_timeout"`
}

// RegisterBackend adds new backend to VaaS if it does not exist.
func (sh *Hook) RegisterBackend(taskInfo mesosutils.TaskInfo) error {
	director := taskInfo.GetLabelValue(vaasDirectorLabelKey)
	if director == "" {
		log.Info("Director not set, skipping registration in VaaS.")
		return nil
	}

	log.Info("Registering backend in VaaS...")

	runtimeDC, err := runenv.Datacenter()

	if err != nil {
		return err
	}

	dc, err := sh.client.GetDC(runtimeDC)

	if err != nil {
		return err
	}

	directorID, err := sh.client.FindDirectorID(director)
	if err != nil {
		return err
	}

	ports := taskInfo.GetPorts()

	if len(ports) < 1 {
		return errors.New("service has no ports available")
	}

	var initialWeight *int
	if weight, err := taskInfo.GetWeight(); err != nil {
		log.WithError(err).Info("VaaS backend weight not set")
	} else {
		initialWeight = &weight
	}

	//TODO(janisz): Remove below code once we find a solution for
	// setting initial weights in labels only.
	initialWeightEnv := taskInfo.FindEnvValue(vaasInitialWeight)
	if val, err := strconv.Atoi(initialWeightEnv); err == nil {
		initialWeight = &val
	}

	// check if it's canary instance - if yes, add new tag "canary" for VaaS
	// (VaaS requires every canary instance to be tagged with "canary" tag)
	// see https://github.com/allegro/vaas/blob/master/docs/documentation/canary.md for details
	isCanary := taskInfo.GetLabelValue(canaryLabelKey)
	var tags []string
	if isCanary != "" {
		tags = []string{canaryLabelKey}
	}

	backend := &Backend{
		Address:            runenv.IP().String(),
		Director:           fmt.Sprintf("%s%d/", apiDirectorPath, directorID),
		Weight:             initialWeight,
		DC:                 *dc,
		Port:               int(ports[0].GetNumber()),
		InheritTimeProfile: true,
		Tags:               tags,
	}

	if taskInfo.GetLabelValue(vaasAsyncLabelKey) == "true" {
		log.Warn("Async VaaS registration is no longer supported")
	}
	_, err = sh.client.AddBackend(backend)
	if err != nil {
		return fmt.Errorf("unable to register backend with VaaS, %s", err)
	}
	sh.backendID = backend.ID

	log.WithField(vaasBackendIDKey, *sh.backendID).Info("Registered backend with VaaS")

	return nil
}

// DeregisterBackend deletes backend from VaaS.
func (sh *Hook) DeregisterBackend(_ mesosutils.TaskInfo) error {
	if sh.backendID != nil {
		log.WithField(vaasBackendIDKey, sh.backendID).
			Info("backendID is set - scheduling backend for deletion via VaaS")

		if err := sh.client.DeleteBackend(*sh.backendID); err != nil {
			return err
		}

		log.WithField(vaasBackendIDKey, sh.backendID).
			Info("Successfully scheduled backend for deletion via VaaS")
		// we will not try to remove the same backend (and get an error) if this hook gets called again
		sh.backendID = nil

		return nil
	}

	log.Infof("backendID not set - not deleting backend from VaaS")

	return nil
}

// HandleEvent calls appropriate hook functions that correspond to supported
// event types. Unsupported events are ignored.
func (sh *Hook) HandleEvent(event hook.Event) (hook.Env, error) {
	switch event.Type {
	case hook.AfterTaskHealthyEvent:
		return nil, sh.RegisterBackend(event.TaskInfo)
	case hook.BeforeTerminateEvent:
		return nil, sh.DeregisterBackend(event.TaskInfo)
	default:
		log.Debugf("Received unsupported event type %s - ignoring", event.Type)
		return nil, nil // ignore unsupported events
	}
}

// NewHook returns new instance of Hook.
func NewHook(cfg Config) (*Hook, error) {
	return &Hook{
		client: NewClient(
			cfg.VaasAPIHost,
			cfg.VaasAPIUsername,
			cfg.VaasAPIKey,
		),
		asyncTimeout: cfg.VaasAsyncTimeout,
	}, nil
}
