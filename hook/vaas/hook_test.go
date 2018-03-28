package vaas

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/mesos/mesos-go/api/v1/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/allegro/mesos-executor/hook"
	"github.com/allegro/mesos-executor/mesosutils"
	"github.com/allegro/mesos-executor/runenv"
)

type MockClient struct {
	mock.Mock
}

func (m *MockClient) FindDirectorID(name string) (int, error) {
	args := m.Called(name)
	return args.Int(0), args.Error(1)
}

func (m *MockClient) AddBackend(backend *Backend) (string, error) {
	args := m.Called(backend)

	backendId := 123
	backend.ID = &backendId
	backend.ResourceURI = "/api/v0.1/backend/123/"

	return args.String(0), args.Error(1)
}

func (m *MockClient) DeleteBackend(id int) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockClient) GetDC(name string) (*DC, error) {
	args := m.Called(name)

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*DC), args.Error(1)
}

func prepareTaskInfo() mesosutils.TaskInfo {
	ports := mesos.Ports{Ports: []mesos.Port{{Number: uint32(8080)}}}
	discovery := mesos.DiscoveryInfo{Ports: &ports}

	return mesosutils.TaskInfo{mesos.TaskInfo{Discovery: &discovery}}
}

func prepareTaskInfoWithDirector(directorName string, extraLabels ...mesos.Label) (taskInfo mesosutils.TaskInfo) {
	taskInfo = prepareTaskInfo()
	tag := "tag"
	directorLabel := mesos.Label{Key: "director", Value: &directorName}
	weightLabel := mesos.Label{Key: "weight:50", Value: &tag}
	labelList := []mesos.Label{directorLabel, weightLabel}
	labelList = append(labelList, extraLabels...)
	labels := mesos.Labels{Labels: labelList}
	taskInfo.TaskInfo.Labels = &labels

	taskInfo.TaskInfo.Command = &mesos.CommandInfo{}

	return taskInfo
}

func TestIfBackendIDSetWhenBackendRegistrationSucceeds(t *testing.T) {
	_ = os.Setenv("CLOUD_DC", "dc6")
	defer os.Unsetenv("CLOUD_DC")

	mockClient := new(MockClient)
	mockDC := DC{
		ID:          1,
		ResourceURI: "dc/6",
	}

	mockClient.On("GetDC", "dc6").Return(&mockDC, nil)
	mockClient.On("FindDirectorID", "abc456").Return(456, nil)
	weight := 50
	mockClient.On("AddBackend", &Backend{
		Address:            runenv.IP().String(),
		DC:                 mockDC,
		Director:           "/api/v0.1/director/456/",
		InheritTimeProfile: true,
		Port:               8080,
		Weight:             &weight,
	}).Return("/api/v0.1/backend/123/", nil)

	serviceHook := Hook{client: mockClient}
	err := serviceHook.RegisterBackend(prepareTaskInfoWithDirector("abc456"))

	require.NoError(t, err)
	expectedId := 123
	assert.Equal(t, &expectedId, serviceHook.backendID)
	mockClient.AssertExpectations(t)
}

func TestBackendRegistrationWhenAddBackendFails(t *testing.T) {
	_ = os.Setenv("CLOUD_DC", "dc6")
	defer os.Unsetenv("CLOUD_DC")

	mockClient := new(MockClient)
	mockDC := DC{
		ID:          1,
		ResourceURI: "dc/6",
	}

	mockClient.On("GetDC", "dc6").Return(&mockDC, nil)
	mockClient.On("FindDirectorID", "abc456").Return(456, nil)
	weight := 50
	mockClient.On("AddBackend", &Backend{
		Address:            runenv.IP().String(),
		DC:                 mockDC,
		Director:           "/api/v0.1/director/456/",
		InheritTimeProfile: true,
		Port:               8080,
		Weight:             &weight,
	}).Return("/api/v0.1/backend/123/", fmt.Errorf("test error"))

	serviceHook := Hook{client: mockClient}
	err := serviceHook.RegisterBackend(prepareTaskInfoWithDirector("abc456"))

	require.EqualError(t, err, "unable to register backend with VaaS, test error")
	mockClient.AssertExpectations(t)
}

func TestIfWeightIsOverriddenByEnvironmentVariable(t *testing.T) {
	_ = os.Setenv("CLOUD_DC", "dc6")
	defer os.Unsetenv("CLOUD_DC")

	mockClient := new(MockClient)
	mockDC := DC{
		ID:          1,
		ResourceURI: "dc/6",
	}

	mockClient.On("GetDC", "dc6").Return(&mockDC, nil)
	mockClient.On("FindDirectorID", "abc456").Return(456, nil)
	weight := 15
	mockClient.On("AddBackend", &Backend{
		Address:            runenv.IP().String(),
		DC:                 mockDC,
		Director:           "/api/v0.1/director/456/",
		InheritTimeProfile: true,
		Port:               8080,
		Weight:             &weight,
	}).Return("/api/v0.1/backend/123/", nil)

	serviceHook := Hook{client: mockClient}

	taskInfo := prepareTaskInfoWithDirector("abc456")
	taskInfo.TaskInfo.Command.Environment = &mesos.Environment{
		Variables: []mesos.Environment_Variable{{"VAAS_INITIAL_WEIGHT", "15"}},
	}

	err := serviceHook.RegisterBackend(taskInfo)

	require.NoError(t, err)
	expectedId := 123
	assert.Equal(t, &expectedId, serviceHook.backendID)
	mockClient.AssertExpectations(t)
}

func TestIfBackendTagsSetWhenCanaryBackendRegistrationSucceeds(t *testing.T) {
	_ = os.Setenv("CLOUD_DC", "dc6")
	defer os.Unsetenv("CLOUD_DC")

	mockClient := new(MockClient)
	mockDC := DC{
		ID:          1,
		ResourceURI: "dc/6",
	}

	mockClient.On("GetDC", "dc6").Return(&mockDC, nil)
	mockClient.On("FindDirectorID", "abc456").Return(456, nil)
	weight := 50
	mockClient.On("AddBackend", &Backend{
		Address:            runenv.IP().String(),
		DC:                 mockDC,
		Director:           "/api/v0.1/director/456/",
		InheritTimeProfile: true,
		Port:               8080,
		Tags:               []string{"canary"},
		Weight:             &weight,
	}).Return("/api/v0.1/backend/123/", nil)

	serviceHook := Hook{client: mockClient}

	tag := "tag"
	err := serviceHook.RegisterBackend(prepareTaskInfoWithDirector("abc456", mesos.Label{Key: "canary", Value: &tag}))

	require.NoError(t, err)
	expectedId := 123
	assert.Equal(t, &expectedId, serviceHook.backendID)
	mockClient.AssertExpectations(t)
}

func TestFailureWhenGettingDCListTimeout(t *testing.T) {
	_ = os.Setenv("CLOUD_DC", "dc6")
	defer os.Unsetenv("CLOUD_DC")

	mockClient := new(MockClient)
	mockClient.On("GetDC", "dc6").Return(nil, errors.New("Connection timeout"))

	serviceHook := Hook{client: mockClient}

	err := serviceHook.RegisterBackend(prepareTaskInfoWithDirector("abc456"))

	require.Error(t, err)
	mockClient.AssertExpectations(t)
}

func TestIfVaasBackendDeleteIsCalledWhenBackendIDSet(t *testing.T) {
	mockClient := new(MockClient)
	backendId := 1324
	mockClient.On("DeleteBackend", backendId).Return(nil)

	serviceHook := Hook{
		backendID: &backendId,
		client:    mockClient,
	}

	err := serviceHook.DeregisterBackend(prepareTaskInfo())

	require.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestDoNotCallVaasBackendDeleteWhenBackendIDEmpty(t *testing.T) {
	mockClient := new(MockClient)
	mockClient.On("DeleteBackend", "").Return(nil)

	serviceHook := Hook{
		client: mockClient,
	}

	err := serviceHook.DeregisterBackend(prepareTaskInfo())

	require.NoError(t, err)
	mockClient.AssertNotCalled(t, "DeleteBackend", "")
}

func TestDoNotRegisterVaasBackendWhenDirectorNotSet(t *testing.T) {

	mockClient := new(MockClient)
	serviceHook := Hook{client: mockClient}

	err := serviceHook.RegisterBackend(prepareTaskInfo())

	assert.Nil(t, err)
}

func TestIfNoErrorOnUnsupportedEvent(t *testing.T) {
	h, err := NewHook(Config{Enabled: true})

	require.NoError(t, err)

	_, err = h.HandleEvent(hook.Event{
		Type: hook.BeforeTaskStartEvent,
	})

	require.NoError(t, err)
}

func TestIfNewHookCreatesNoopHookWhenHookDisabled(t *testing.T) {
	h, err := NewHook(Config{Enabled: false})

	require.NoError(t, err)
	assert.IsType(t, hook.NoopHook{}, h)
}