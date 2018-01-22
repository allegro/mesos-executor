package xnet

import (
	"fmt"
	"net"
	"strconv"
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
)

func TestIntegrationWithConsulRoundRobinAndNetworkSend(t *testing.T) {
	// start consul for discovery service
	config, server := createTestConsulServer(t)
	consulApiClient, err := api.NewClient(config)
	require.NoError(t, err)
	defer stopConsul(server)

	// create listener acting as a service
	addr, result, err := udpServer()
	require.NoError(t, err)
	host, portString, err := net.SplitHostPort(string(addr))
	require.NoError(t, err)
	port, _ := strconv.Atoi(portString)

	// register service in consul
	agent := consulApiClient.Agent()
	err = agent.ServiceRegister(&api.AgentServiceRegistration{
		ID:      "1",
		Name:    "service-name",
		Port:    port,
		Address: host,
	})
	require.NoError(t, err)

	sender := &UDPSender{}

	// create round robin writer writing by network and obtaining instances provided by consul
	writer := RoundRobinWriter(
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

func TestUDPNetworkSendShouldReturnErrorWhenConnectionUnavailable(t *testing.T) {
	sender := &UDPSender{}

	bytesSent, err := sender.Send("198.51.100.5", []byte("test")) // see RFC 5737 for more info about this IP address

	assert.Error(t, err)
	assert.Zero(t, bytesSent)
}

func TestUDPNetworkSendShouldReturnNumberOfSentBytes(t *testing.T) {
	addr, result, err := udpServer()
	require.NoError(t, err)

	sender := &UDPSender{}

	bytesSent, err := sender.Send(addr, []byte(testPayload))

	assert.NoError(t, err)
	assert.Equal(t, len(testPayload), bytesSent)
	assert.Equal(t, testPayload, <-result)
}

func udpServer() (Address, <-chan string, error) {
	udpAddr := net.UDPAddr{
		IP: net.IPv4(127, 0, 0, 1),
	}
	conn, err := net.ListenUDP("udp4", &udpAddr)
	if err != nil {
		return "", nil, err
	}
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

	return Address(conn.LocalAddr().String()), result, nil
}

func TestDiscoveryServiceInstanceProviderShouldPeriodicallyUpdatesInstances(t *testing.T) {
	ret := make(chan []Address, 3)
	client := &StubDiscoveryServiceClient{returns: ret}

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
	ret := make(chan []Address, 1)
	client := &StubDiscoveryServiceClient{returns: ret}

	// setup expectations
	ret <- []Address{}

	provider := DiscoveryServiceInstanceProvider("service name", 1, client)

	assert.Equal(t, []Address{}, <-provider)
	assert.Empty(t, ret)
}

func TestIfUpdatesAddressesOnlyIfTheyChanged(t *testing.T) {
	returns := make(chan []Address, 5)
	discoveryServiceClient := &StubDiscoveryServiceClient{returns}

	// setup expectations
	returns <- []Address{"127.0.0.1:1234", "127.0.0.1:4321"} // initial instances
	returns <- []Address{"127.0.0.1:1234", "127.0.0.1:4321"} // same as before but different order
	returns <- []Address{"127.0.0.1:1234", "127.0.0.1:4321"} // same as before
	returns <- []Address{"127.0.0.1:4321", "127.0.0.1:1234"} // same as before
	returns <- []Address{"127.0.0.1:5678", "127.0.0.1:8765"} // different ports

	instanceProvider := DiscoveryServiceInstanceProvider("service-name", 1, discoveryServiceClient)

	assert.Equal(t, []Address{"127.0.0.1:1234", "127.0.0.1:4321"}, <-instanceProvider)
	assert.Equal(t, []Address{"127.0.0.1:5678", "127.0.0.1:8765"}, <-instanceProvider)
	assert.Empty(t, returns)
}

func TestRoundRobinShouldWalkThruAllElementsWhenNoUpdate(t *testing.T) {
	provider := make(chan []Address, 2)
	provider <- []Address{"1", "2", "3"}

	sender := &MockSender{}
	sender.On("Send", Address("1"), []byte("x")).Return(1, nil).Twice()
	sender.On("Send", Address("2"), []byte("x")).Return(1, nil).Twice()
	sender.On("Send", Address("3"), []byte("x")).Return(1, nil).Twice()
	sender.On("Release").Return(nil)

	writer := RoundRobinWriter(provider, sender)

	for i := 0; i < 6; i++ {
		_, err := writer.Write([]byte("x"))
		assert.NoError(t, err)
	}

	sender.AssertExpectations(t)
}

func TestRoundRobinShouldStartFromTheBegginingAfterUpdate(t *testing.T) {
	provider := make(chan []Address, 3)
	provider <- []Address{"1", "2", "3"}

	sender := &MockSender{}
	sender.On("Send", Address("1"), []byte("x")).Return(1, nil).Times(2)
	sender.On("Release").Return(nil)

	writer := RoundRobinWriter(provider, sender)

	_, err := writer.Write([]byte("x"))
	assert.NoError(t, err)

	provider <- []Address{"1", "2", "4"}

	_, err = writer.Write([]byte("x"))

	assert.NoError(t, err)
	sender.AssertExpectations(t)
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
}

func (m ErrorDiscoveryServiceClient) GetAddrsByName(serviceName string) ([]Address, error) {
	return nil, fmt.Errorf("error")
}

type StubDiscoveryServiceClient struct {
	returns chan []Address
}

func (m *StubDiscoveryServiceClient) GetAddrsByName(serviceName string) ([]Address, error) {
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
		t.Run(fmt.Sprintf("serviceName=%s", tc.name), func(t *testing.T) {
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

type MockSender struct {
	mock.Mock
}

func (s *MockSender) Send(addr Address, payload []byte) (int, error) {
	args := s.Called(addr, payload)
	return args.Int(0), args.Error(1)
}

func (s *MockSender) Release() error {
	args := s.Called()
	return args.Error(0)
}
