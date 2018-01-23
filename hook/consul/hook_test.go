package consul

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/testutil"
	mesos "github.com/mesos/mesos-go/api/v1/lib"
	"github.com/stretchr/testify/require"

	"github.com/allegro/mesos-executor/hook"
	"github.com/allegro/mesos-executor/mesosutils"
)

func TestIfUsesLabelledPortsForServiceIDGen(t *testing.T) {
	consulName := "consulName"
	taskID := "taskID"
	taskInfo := prepareTaskInfo(taskID, consulName, consulName, []string{"metrics", "otherTag"}, []mesos.Port{
		{Number: 666},
		{
			Number: 997,
			Labels: &mesos.Labels{
				Labels: []mesos.Label{
					{
						Key:   "consul",
						Value: &consulName,
					},
				},
			},
		},
	})

	expectedServiceID := createServiceId(taskID, consulName, 997)

	// Create a test Consul server
	config, server := createTestConsulServer(t)
	client, _ := api.NewClient(config) // #nosec
	defer stopConsul(server)

	h := &Hook{client: client}
	err := h.RegisterIntoConsul(taskInfo)

	require.NoError(t, err)
	require.Len(t, h.serviceInstances, 1)
	require.Equal(t, expectedServiceID, h.serviceInstances[0].consulServiceID)

	opts := api.QueryOptions{}
	services, _, err := client.Catalog().Services(&opts)
	require.NoError(t, err)
	require.Contains(t, services, consulName)
	require.Contains(t, services[consulName], "metrics")
	require.Contains(t, services[consulName], "otherTag")
}

func TestIfUsesFirstPortIfNoneIsLabelledForServiceIDGen(t *testing.T) {
	consulName := "consulName"
	taskID := "taskID"
	taskInfo := prepareTaskInfo(taskID, consulName, consulName, []string{"metrics"}, []mesos.Port{
		{Number: 666},
		{Number: 997},
	})

	expectedServiceID := createServiceId(taskID, consulName, 666)

	// Create a test Consul server
	config, server := createTestConsulServer(t)
	client, _ := api.NewClient(config) // #nosec
	defer stopConsul(server)

	h := &Hook{client: client}
	err := h.RegisterIntoConsul(taskInfo)

	require.NoError(t, err)
	require.Len(t, h.serviceInstances, 1)
	require.Equal(t, expectedServiceID, h.serviceInstances[0].consulServiceID)

	opts := api.QueryOptions{}
	services, _, err := client.Catalog().Services(&opts)
	require.NoError(t, err)
	require.Contains(t, services, consulName)
}

func TestIfUsesLabelledPortsForServiceIDGenAndRegisterMultiplePorts(t *testing.T) {
	consulNameFirst := "consulName"
	consulNameSecond := "consulName-secured"
	taskID := "taskID"
	taskInfo := prepareTaskInfo(taskID, consulNameFirst, consulNameFirst, []string{"metrics"}, []mesos.Port{
		{
			Number: 998,
			Labels: &mesos.Labels{
				Labels: []mesos.Label{
					{
						Key:   "consul",
						Value: &consulNameSecond,
					},
				},
			},
		},
		{Number: 666},
		{
			Number: 997,
			Labels: &mesos.Labels{
				Labels: []mesos.Label{
					{
						Key:   "consul",
						Value: &consulNameFirst,
					},
				},
			},
		},
	})
	expectedServiceID := createServiceId(taskID, consulNameFirst, 997)
	expectedServiceID2 := createServiceId(taskID, consulNameSecond, 998)

	// Create a test Consul server
	config, server := createTestConsulServer(t)
	client, _ := api.NewClient(config) // #nosec
	defer stopConsul(server)

	h := &Hook{client: client}
	err := h.RegisterIntoConsul(taskInfo)

	require.NoError(t, err)
	require.Len(t, h.serviceInstances, 2)
	require.Equal(t, expectedServiceID2, h.serviceInstances[0].consulServiceID)
	require.Equal(t, expectedServiceID, h.serviceInstances[1].consulServiceID)

	opts := api.QueryOptions{}
	services, _, err := client.Catalog().Services(&opts)
	require.NoError(t, err)
	require.Contains(t, services, consulNameFirst)
	require.Contains(t, services, consulNameSecond)
}

func TestIfUsesPortLabelsForRegistration(t *testing.T) {
	consulName := "consulName"
	consulNameSecond := "consulName-secured"
	taskID := "taskID"
	tagValue := "tag"
	taskInfo := prepareTaskInfo(taskID, consulName, consulName, []string{"metrics", "extras"}, []mesos.Port{
		{
			Number: 666,
			Labels: &mesos.Labels{
				Labels: []mesos.Label{
					{
						Key:   "consul",
						Value: &consulName,
					},
					{
						Key:   "hystrix",
						Value: &tagValue,
					},
				},
			},
		},
		{
			Number: 997,
			Labels: &mesos.Labels{
				Labels: []mesos.Label{
					{
						Key:   "consul",
						Value: &consulNameSecond,
					},
				},
			},
		},
	})

	expectedService := instance{
		consulServiceName: "consulName",
		consulServiceID:   createServiceId(taskID, consulName, 666),
		port:              666,
		tags:              []string{"hystrix", "metrics", "extras", "marathon"},
	}
	expectedService2 := instance{
		consulServiceName: "consulName-secured",
		consulServiceID:   createServiceId(taskID, consulNameSecond, 997),
		port:              997,
		tags:              []string{"metrics", "extras", "marathon"},
	}

	// Create a test Consul server
	config, server := createTestConsulServer(t)
	client, _ := api.NewClient(config) // #nosec
	defer stopConsul(server)

	h := &Hook{client: client}
	err := h.RegisterIntoConsul(taskInfo)

	require.NoError(t, err)
	require.Len(t, h.serviceInstances, 2)
	require.Equal(t, []instance{expectedService, expectedService2}, h.serviceInstances)

	opts := api.QueryOptions{}
	services, _, err := client.Catalog().Services(&opts)
	require.NoError(t, err)
	require.Contains(t, services, consulName)
	requireEqualElements(t, expectedService.tags, services[consulName])
	require.Contains(t, services, consulNameSecond)
	requireEqualElements(t, expectedService2.tags, services[consulNameSecond])
}

// requireEqualElements asserts that two slices are equal ignoring order of elements
func requireEqualElements(t *testing.T, expected, actual []string) {
	require.Len(t, actual, len(expected))
	for _, element := range expected {
		require.Contains(t, actual, element)
	}
}

func TestIfGeneratesNameIfConsulLabelTrueOrEmpty(t *testing.T) {
	taskID := "taskID"
	taskName := "consulName"

	labelValues := []string{"true", ""}

	for _, labelValue := range labelValues {
		taskInfo := prepareTaskInfo(taskID, taskName, labelValue, []string{"metrics", "otherTag"}, []mesos.Port{
			{Number: 666},
		})

		expectedServiceID := createServiceId(taskID, taskID, 666)

		// Create a test Consul server
		config, server := createTestConsulServer(t)
		client, _ := api.NewClient(config) // #nosec
		defer stopConsul(server)

		h := &Hook{client: client}
		err := h.RegisterIntoConsul(taskInfo)

		require.NoError(t, err)
		require.Len(t, h.serviceInstances, 1)
		require.Equal(t, expectedServiceID, h.serviceInstances[0].consulServiceID)

		opts := api.QueryOptions{}
		services, _, err := client.Catalog().Services(&opts)
		require.NoError(t, err)
		require.Contains(t, services, taskID)
		require.Contains(t, services[taskID], "metrics")
		require.Contains(t, services[taskID], "otherTag")
	}
}

func TestIfGeneratesCorrectNameIfConsulLabelEmpty(t *testing.T) {
	t.Parallel()

	var intentNameTestsData = []struct {
		taskID       string
		expectedName string
	}{
		{"/rootGroup/subGroup/subSubGroup/name", "rootGroup-subGroup-subSubGroup-name"},
		{"com.examle_examle-app.4646218e-a9b7-11e7-938f-02c89eb9127",
			"com.examle.examle-app"},
	}

	for _, testData := range intentNameTestsData {
		t.Run(fmt.Sprintf("%s translates to %s", testData.taskID, testData.expectedName), func(t *testing.T) {
			taskInfo := prepareTaskInfo(testData.taskID, testData.expectedName, "",
				[]string{"metrics", "otherTag"}, []mesos.Port{
					{Number: 666},
				})

			expectedServiceID := createServiceId(testData.taskID, testData.expectedName, 666)

			// Create a test Consul server
			config, server := createTestConsulServer(t)
			client, _ := api.NewClient(config) // #nosec
			defer stopConsul(server)

			h := &Hook{client: client}
			err := h.RegisterIntoConsul(taskInfo)

			require.NoError(t, err)
			require.Len(t, h.serviceInstances, 1)
			require.Equal(t, expectedServiceID, h.serviceInstances[0].consulServiceID)

			opts := api.QueryOptions{}
			services, _, err := client.Catalog().Services(&opts)
			require.NoError(t, err)
			require.Contains(t, services, testData.expectedName)
		})
	}
}

func TestIfNoErrorOnUnsupportedEvent(t *testing.T) {
	h, err := NewHook(Config{})

	require.NoError(t, err)

	_, err = h.HandleEvent(hook.Event{
		Type: hook.BeforeTaskStartEvent,
	})

	require.NoError(t, err)
}

func TestIfServiceDeregisteredCorrectly(t *testing.T) {
	consulName := "consulName"
	taskID := "taskID"
	taskInfo := prepareTaskInfo(taskID, consulName, consulName, []string{"metrics"}, []mesos.Port{
		{Number: 777},
	})

	// Create a test Consul server
	config, server := createTestConsulServer(t)
	client, _ := api.NewClient(config) // #nosec
	defer stopConsul(server)

	h := &Hook{client: client}
	err := h.RegisterIntoConsul(taskInfo)

	require.NoError(t, err)
	require.Len(t, h.serviceInstances, 1)

	opts := api.QueryOptions{}
	services, _, err := client.Catalog().Services(&opts)
	require.NoError(t, err)
	require.Contains(t, services, consulName)

	err = h.DeregisterFromConsul(taskInfo)

	require.NoError(t, err)
	services, _, err = client.Catalog().Services(&opts)
	require.NoError(t, err)
	require.NotContains(t, services, consulName)
}

func TestIfErrorHandledOnNoConsul(t *testing.T) {
	consulName := "consulName"
	taskID := "taskID"
	taskInfo := prepareTaskInfo(taskID, consulName, consulName, []string{"metrics"}, []mesos.Port{
		{Number: 666},
	})

	config := api.DefaultConfig()
	config.Address = "http://localhost:5200"
	client, _ := api.NewClient(config) // #nosec

	h := &Hook{client: client}
	err := h.RegisterIntoConsul(taskInfo)

	require.Error(t, err)
	require.Len(t, h.serviceInstances, 0)
}

func stopConsul(server *testutil.TestServer) {
	_ = server.Stop()
}

func createServiceId(taskId string, taskName string, port int) string {
	return taskId + "_" + taskName + "_" + strconv.Itoa(port)
}

func prepareTaskInfo(taskID string, taskName string, consulName string, tags []string, ports []mesos.Port) mesosutils.TaskInfo {
	seconds := 5.0
	path := "/"
	tagName := "tag"

	labels := []mesos.Label{
		{
			Key:   "consul",
			Value: &consulName,
		},
	}
	for _, tag := range tags {
		labels = append(labels, mesos.Label{
			Key:   tag,
			Value: &tagName,
		})
	}

	healthPort := ports[0].GetNumber()

	return mesosutils.TaskInfo{mesos.TaskInfo{
		Discovery: &mesos.DiscoveryInfo{
			Ports: &mesos.Ports{
				Ports: ports,
			},
		},
		Labels: &mesos.Labels{
			Labels: labels,
		},
		Name: taskName,
		TaskID: mesos.TaskID{
			Value: taskID,
		},
		HealthCheck: &mesos.HealthCheck{
			IntervalSeconds: &seconds,
			TimeoutSeconds:  &seconds,
			HTTP: &mesos.HealthCheck_HTTPCheckInfo{
				Port: healthPort,
				Path: &path,
			},
		},
	}}
}

func createTestConsulServer(t *testing.T) (config *api.Config, server *testutil.TestServer) {
	server, err := testutil.NewTestServer()
	if err != nil {
		t.Fatal(err)
	}

	config = api.DefaultConfig()
	config.Address = server.HTTPAddr
	return config, server
}
