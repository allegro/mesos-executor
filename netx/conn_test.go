package netx

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	testPayload = "test"
	loopback    = "127.0.0.1"
)

func TestIntegrationWithConsulRoundRobinAndNetworkSend(t *testing.T) {

	// start consul for discovery service
	config, server := createTestConsulServer(t)
	consulApiClient, err := api.NewClient(config)
	require.NoError(t, err)
	defer stopConsul(server)

	// create listener acting as a service
	port, result, err := udpServer()
	require.NoError(t, err)

	// register service in consul
	agent := consulApiClient.Agent()
	err = agent.ServiceRegister(&api.AgentServiceRegistration{
		ID:      "1",
		Name:    "service-name",
		Port:    port,
		Address: loopback,
	})
	require.NoError(t, err)

	sender, err := UDPSender()
	require.NoError(t, err)

	// create round robin writer writing by network and obtaining instances provided by consul
	writer := RoundRobin(
		DiscoveryServiceInstanceProvider(
			"service-name",
			time.Second,
			NewConsulDiscoveryServiceClient(consulApiClient)),
		sender)

	// write something
	bytesSent, err := writer.Write([]byte(testPayload))

	assert.NoError(t, err)
	assert.Equal(t, len(testPayload), bytesSent)
	assert.Equal(t, testPayload, <-result)
}

func TestNetworkSendShouldReturnErrorWhenConnectionUnavailable(t *testing.T) {
	sender, err := UDPSender()
	require.NoError(t, err)

	bytesSent, err := sender(loopback, []byte("test"))

	assert.Error(t, err)
	assert.Zero(t, bytesSent)
}

func TestNetworkSendShouldReturnNumberOfSentBytes(t *testing.T) {
	port, result, err := udpServer()
	require.NoError(t, err)

	sender, err := UDPSender()
	require.NoError(t, err)

	bytesSent, err := sender(localhost(port), []byte(testPayload))

	assert.NoError(t, err)
	assert.Equal(t, len(testPayload), bytesSent)
	assert.Equal(t, testPayload, <-result)
}

func TestUDPSenderWithSharedConnShouldReturnNumberOfSentBytes(t *testing.T) {
	port, result, err := udpServer()
	require.NoError(t, err)

	sender, err := UDPSender()
	require.NoError(t, err)

	bytesSent, err := sender(localhost(port), []byte(testPayload))

	assert.NoError(t, err)
	assert.Equal(t, len(testPayload), bytesSent)
	assert.Equal(t, testPayload, <-result)
}

func udpServer() (int, <-chan string, error) {
	udpAddr := net.UDPAddr{}
	conn, err := net.ListenUDP("udp", &udpAddr)
	if err != nil {
		return 0, nil, err
	}

	udpAddr, _ = addressToUDP(Address(conn.LocalAddr().String()))

	result := make(chan string)

	go func() {
		defer conn.Close()
		buf := make([]byte, 1024)
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			result <- err.Error()
			return
		}

		result <- string(buf[0:n])
	}()

	return udpAddr.Port, result, nil
}

func TestDiscoveryServiceInstanceProviderShouldPeriodicallyUpdatesInstances(t *testing.T) {
	ret := make(chan []Address, 3)
	client := &MockDiscoveryServiceClient{returns: ret}

	// setup expectations
	ret <- []Address{"192.0.2.1:80"}
	ret <- []Address{"192.0.2.2:80"}
	ret <- []Address{"192.0.2.3:80"}

	provider := DiscoveryServiceInstanceProvider("service name", 1, client)

	assert.Equal(t, []Address{"192.0.2.1:80"}, <-provider)
	assert.Equal(t, []Address{"192.0.2.2:80"}, <-provider)
	assert.Equal(t, []Address{"192.0.2.3:80"}, <-provider)
	assert.Empty(t, ret)
}

func TestDiscoveryServiceInstanceProviderShouldUpdateInstancesWhenTheyAreEmpty(t *testing.T) {
	ret := make(chan []Address, 3)
	client := &MockDiscoveryServiceClient{returns: ret}

	// setup expectations
	ret <- []Address{}
	ret <- []Address{}
	ret <- []Address{}

	provider := DiscoveryServiceInstanceProvider("service name", 1, client)

	assert.Equal(t, []Address{}, <-provider)
	assert.Equal(t, []Address{}, <-provider)
	assert.Equal(t, []Address{}, <-provider)
	assert.Empty(t, ret)
}

func TestRoundRobinShouldWalkThruAllElementsWhenNoUpdate(t *testing.T) {

	provider := make(chan []Address, 2)
	provider <- []Address{"1", "2", "3"}

	var addresses []Address

	sender := func(addr Address, payload []byte) (int, error) {
		addresses = append(addresses, addr)
		return len(payload), nil
	}

	writer := RoundRobin(provider, sender)

	for i := 0; i < 6; i++ {
		_, err := writer.Write(nil)
		assert.NoError(t, err)
	}

	assert.Equal(t, []Address{
		"1", "2", "3",
		"1", "2", "3"},
		addresses)
}

func TestRoundRobinShouldStartFromTheBegginingAfterUpdate(t *testing.T) {

	provider := make(chan []Address, 3)
	provider <- []Address{"1", "2", "3"}

	var addresses []Address

	sender := func(addr Address, payload []byte) (int, error) {
		addresses = append(addresses, addr)
		return len(payload), nil
	}

	writer := RoundRobin(provider, sender)

	_, err := writer.Write(nil)
	assert.NoError(t, err)

	provider <- []Address{"1", "2", "3"}

	_, err = writer.Write(nil)
	assert.NoError(t, err)

	assert.Equal(t, []Address{"1", "1"}, addresses)
}

func TestDiscoveryServiceInstanceProviderShouldNotUpdateWithEmptyInstancesOnError(t *testing.T) {
	client := ErrorDiscoveryServiceClient{}

	provider := DiscoveryServiceInstanceProvider("service name", 1, client)

	select {
	case <-provider:
		t.Errorf("Should wait forever!")
	case <-time.Tick(time.Millisecond):
	}
}

type ErrorDiscoveryServiceClient struct {
	mock.Mock
}

func (m ErrorDiscoveryServiceClient) GetAddrsByName(serviceName string) ([]Address, error) {
	return nil, fmt.Errorf("error")
}

type MockDiscoveryServiceClient struct {
	returns chan []Address
}

func (m *MockDiscoveryServiceClient) GetAddrsByName(serviceName string) ([]Address, error) {
	return <-m.returns, nil
}

func TestIfGetAddrsByNameReturnsOnlyMatchingInstancesFromConsul(t *testing.T) {
	config, server := createTestConsulServer(t)
	consulApiClient, err := api.NewClient(config)
	defer stopConsul(server)

	require.NoError(t, err)

	// given
	agent := consulApiClient.Agent()
	for id, name := range []string{"A", "A", "B", "B", "B", "C"} {
		err = agent.ServiceRegister(registration(id, name))
		require.NoError(t, err)
	}

	client := consulDiscoveryServiceClient{client: consulApiClient}

	testCases := []struct {
		name string
		want []Address
	}{
		{"A", []Address{"192.0.2.0:0", "192.0.2.1:1"}},
		{"B", []Address{"192.0.2.2:2", "192.0.2.3:3", "192.0.2.4:4"}},
		{"C", []Address{"192.0.2.5:5"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			addr, err := client.GetAddrsByName(tc.name)

			assert.NoError(t, err)
			assert.Equal(t, tc.want, addr)
		})
	}
}

func TestIfGetAddrsByNameReturnsEmptyListIfNoMatchingInstancesInConsul(t *testing.T) {
	config, server := createTestConsulServer(t)
	consulApiClient, err := api.NewClient(config)
	defer stopConsul(server)

	require.NoError(t, err)

	client := consulDiscoveryServiceClient{client: consulApiClient}

	// when
	addr, err := client.GetAddrsByName("X")
	// then
	assert.NoError(t, err)
	assert.Empty(t, addr)
}

func TestIfGetAddrsByNameReturnsErrorIfNoConsulConnection(t *testing.T) {
	consulApiClient, err := api.NewClient(api.DefaultConfig())

	require.NoError(t, err)

	client := consulDiscoveryServiceClient{client: consulApiClient}

	// when
	addr, err := client.GetAddrsByName("X")
	// then
	assert.Error(t, err)
	assert.Empty(t, addr)
}

func registration(id int, name string) *api.AgentServiceRegistration {
	return &api.AgentServiceRegistration{
		ID:                fmt.Sprint(id),
		Name:              name,
		Tags:              nil,
		Port:              id,
		Address:           fmt.Sprintf("192.0.2.%d", id),
		EnableTagOverride: false,
	}
}

func stopConsul(server *testutil.TestServer) {
	_ = server.Stop()
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

func localhost(port int) Address {
	return Address(fmt.Sprintf("%s:%d", loopback, port))
}
