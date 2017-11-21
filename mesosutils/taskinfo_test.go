package mesosutils

import (
	"testing"

	mesos "github.com/mesos/mesos-go/api/v1/lib"
	"github.com/stretchr/testify/require"
)

func TestIfExtractsServiceIDFromLabel(t *testing.T) {
	serviceIDLabelValue := "XXX"
	mesosTaskInfo := mesos.TaskInfo{
		Executor: &mesos.ExecutorInfo{
			ExecutorID: mesos.ExecutorID{
				Value: "executorID",
			},
		},
		Labels: &mesos.Labels{
			Labels: []mesos.Label{
				{
					Key:   "serviceId",
					Value: &serviceIDLabelValue,
				},
			},
		},
	}
	taskInfo := TaskInfo{
		TaskInfo: mesosTaskInfo,
	}

	extractedServiceID := taskInfo.GetServiceID()

	require.Equal(t, serviceIDLabelValue, extractedServiceID)
}

func TestIfExtractsValueFromLabel(t *testing.T) {
	label := "vip"
	labelValue := "vip123"
	mesosTaskInfo := mesos.TaskInfo{
		Labels: &mesos.Labels{
			Labels: []mesos.Label{
				{
					Key:   label,
					Value: &labelValue,
				},
			},
		},
	}
	taskInfo := TaskInfo{
		TaskInfo: mesosTaskInfo,
	}

	extractedValue := taskInfo.GetLabelValue(label)

	require.Equal(t, labelValue, extractedValue)
}

func TestIfFindsLabel(t *testing.T) {
	label := "vip"
	labelValue := "vip123"
	mesosTaskInfo := mesos.TaskInfo{
		Labels: &mesos.Labels{
			Labels: []mesos.Label{
				{
					Key:   label,
					Value: &labelValue,
				},
			},
		},
	}
	taskInfo := TaskInfo{
		TaskInfo: mesosTaskInfo,
	}

	found := taskInfo.FindLabel(label)

	require.NotNil(t, found)
	require.Equal(t, found.Value, &labelValue)
	require.Equal(t, found.Key, label)
}

func TestIfExtractsLabelsByValue(t *testing.T) {
	expectedValue := "tag"
	otherValue := "other"
	labelFirst := "labelOne"
	labelSecond := "labelTwo"
	labelThird := "labelThree"
	labels := []mesos.Label{
		{
			Key:   labelFirst,
			Value: &expectedValue,
		},
		{
			Key:   labelSecond,
			Value: &otherValue,
		},
		{
			Key:   labelThird,
			Value: &expectedValue,
		},
	}

	result := GetLabelKeysByValue(labels, expectedValue)

	require.Contains(t, result, labelFirst)
	require.Contains(t, result, labelThird)
	require.NotContains(t, result, labelSecond)
}

func TestIfFallsBackToExecutorIDIfServiceIDLabelMissing(t *testing.T) {
	mesosTaskInfo := mesos.TaskInfo{
		Executor: &mesos.ExecutorInfo{
			ExecutorID: mesos.ExecutorID{
				Value: "executorID",
			},
		},
	}
	taskInfo := TaskInfo{
		TaskInfo: mesosTaskInfo,
	}

	serviceID := taskInfo.GetServiceID()

	require.Equal(t, "executorID", serviceID)
}

func TestGetWeightIfReturnsErrorIfNoWeightDefined(t *testing.T) {
	label := "vip"
	labelValue := "vip123"
	mesosTaskInfo := mesos.TaskInfo{
		Labels: &mesos.Labels{
			Labels: []mesos.Label{
				{
					Key:   label,
					Value: &labelValue,
				},
			},
		},
	}
	taskInfo := TaskInfo{
		TaskInfo: mesosTaskInfo,
	}

	weight, err := taskInfo.GetWeight()

	require.Equal(t, 0, weight)
	require.Error(t, err)
}

func TestGetWeightIfReturnsErrorIfWeightIsNaN(t *testing.T) {
	label := "weight:nan"
	labelValue := "tag"
	mesosTaskInfo := mesos.TaskInfo{
		Labels: &mesos.Labels{
			Labels: []mesos.Label{
				{
					Key:   label,
					Value: &labelValue,
				},
			},
		},
	}
	taskInfo := TaskInfo{
		TaskInfo: mesosTaskInfo,
	}

	weight, err := taskInfo.GetWeight()

	require.Equal(t, 0, weight)
	require.Error(t, err)
}

func TestGetWeightReturnsWeightFromTagLabel(t *testing.T) {
	label := "weight:50"
	labelValue := "tag"
	mesosTaskInfo := mesos.TaskInfo{
		Labels: &mesos.Labels{
			Labels: []mesos.Label{
				{
					Key:   label,
					Value: &labelValue,
				},
			},
		},
	}
	taskInfo := TaskInfo{
		TaskInfo: mesosTaskInfo,
	}

	weight, err := taskInfo.GetWeight()

	require.Equal(t, 50, weight)
	require.NoError(t, err)
}
