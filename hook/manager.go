package hook

import log "github.com/Sirupsen/logrus"

// Manager is a helper type that simplifies calling group of hooks and handling
// returned errors.
type Manager struct {
	Hooks []Hook
}

// HandleEvent calls group of hooks sequentially. It returns error on first hook
// call error when ignoreErrors argument is false. When ignoreErrors is set to
// true it will only log errors returned from each hook and will never return an
// error itself.
func (m *Manager) HandleEvent(event Event, ignoreErrors bool) error {
	for _, hook := range m.Hooks {
		log.Infof("Calling %T hook to handle %s", hook, event.Type)
		if err := hook.HandleEvent(event); err != nil {
			if !ignoreErrors {
				return err
			}
			log.WithError(err).Errorf("%T hook failed to handle %s", hook, event.Type)
		}
	}

	return nil
}
