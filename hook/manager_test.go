package hook

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestIfFailsOnFirstError(t *testing.T) {
	testErr := errors.New("test")
	hook1 := new(mockHook)
	hook1.On("HandleEvent", mock.AnythingOfType("hook.Event")).Return(Env{}, testErr).Once()

	hook2 := new(mockHook)
	hook2.On("HandleEvent", mock.AnythingOfType("hook.Event")).Return(Env{}, testErr).Once()

	manager := Manager{Hooks: []Hook{hook1, hook2}}
	_, err := manager.HandleEvent(Event{}, false)

	assert.Error(t, err)
	hook1.AssertExpectations(t)
	hook2.AssertNotCalled(t, "HandleEvent")
}

func TestIfIgnoresErrors(t *testing.T) {
	testErr := errors.New("test")
	hook1 := new(mockHook)
	hook1.On("HandleEvent", mock.AnythingOfType("hook.Event")).Return(Env{}, testErr).Once()

	hook2 := new(mockHook)
	hook2.On("HandleEvent", mock.AnythingOfType("hook.Event")).Return(Env{}, nil).Once()

	manager := Manager{Hooks: []Hook{hook1, hook2}}
	_, err := manager.HandleEvent(Event{}, true)

	assert.NoError(t, err)
	hook1.AssertExpectations(t)
	hook2.AssertExpectations(t)
}

type mockHook struct {
	mock.Mock
}

func (m *mockHook) HandleEvent(event Event) (Env, error) {
	args := m.Called(event)
	return args.Get(0).(Env), args.Error(1)
}
