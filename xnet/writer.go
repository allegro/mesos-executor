package xnet

import (
	"fmt"
	"io"
	"net"
	"reflect"
	"sort"
	"time"

	"github.com/hashicorp/consul/api"
	log "github.com/sirupsen/logrus"
)

// InstanceProvider is the channel where updated list of desired service are
// published.
type InstanceProvider <-chan []Address

// Sender is a interface for different sending protocols through the network.
type Sender interface {
	// Send writes given payload to passed address. It returns number of bytes
	// sent and error - if there was any.
	Send(Address, []byte) (int, error)

	// Release frees all system resources allocated by the Sender. It can be
	// called many times - usually when the pool of used addresses changes.
	Release() error
}

// Address of a service in IP:PORT format
type Address string

// RoundRobinWriter returns writer with round robin functionality. Every write
// could be sent to different backend.
func RoundRobinWriter(instanceProvider InstanceProvider, sender Sender) io.Writer {
	return &roundRobinWriter{provider: instanceProvider, sender: sender, instances: nil}
}

type roundRobinWriter struct {
	provider  InstanceProvider
	sender    Sender
	instances chan Address
}

func (r *roundRobinWriter) Write(byte []byte) (int, error) {
	if r.instances == nil {
		r.updateInstances(<-r.provider)
	}

	select {
	case newInstances := <-r.provider:
		r.updateInstances(newInstances)
		return r.write(byte)
	default:
		return r.write(byte)
	}
}

func (r *roundRobinWriter) updateInstances(newInstances []Address) {
	r.instances = make(chan Address, len(newInstances))
	for _, instance := range newInstances {
		r.instances <- instance
	}
	if err := r.sender.Release(); err != nil {
		log.WithError(err).Warn("Unable to release xnet.Sender resources")
	}
}

func (r *roundRobinWriter) write(payload []byte) (int, error) {
	// Read next instance from queue
	instance := <-r.instances
	// Enqueue instance for round robin behaviour
	r.instances <- instance

	return r.sender.Send(instance, payload)
}

// UDPSender is a Sender implementation that can write payload to the network
// address and reuses single system socket. It uses UDP packets to send data.
type UDPSender struct {
	conn *net.UDPConn
}

// Send sends given payload to passed address. Data is sent using UDP packets.
// It returns number of bytes sent and error - if there was any.
func (s *UDPSender) Send(addr Address, payload []byte) (int, error) {
	if s.conn == nil {
		conn, err := net.ListenUDP("udp", nil)
		if err != nil {
			return 0, fmt.Errorf("could not create connection: %s", err)
		}
		s.conn = conn
	}

	udpAddr, err := net.ResolveUDPAddr("udp", string(addr))
	if err != nil {
		return 0, fmt.Errorf("invalid address %s: %s", addr, err)
	}

	n, err := s.conn.WriteTo(payload, udpAddr)
	if err != nil {
		return 0, fmt.Errorf("could not sent payload to %s: %s", addr, err)
	}
	return n, nil
}

// Release frees system socket used by sender.
func (s *UDPSender) Release() error {
	if s.conn == nil {
		return nil
	}
	err := s.conn.Close()
	s.conn = nil
	return err
}

// DiscoveryServiceInstanceProvider returns InstanceProvider that is updated with
// list of instances in interval
func DiscoveryServiceInstanceProvider(serviceName string, interval time.Duration, client DiscoveryServiceClient) InstanceProvider {
	instancesChan := make(chan []Address)

	go func() {
		var currInstances []Address
		for range time.NewTicker(interval).C {
			newInstances, err := client.GetAddrsByName(serviceName)
			if err != nil {
				log.WithError(err).Warn("Unable to get newInstances from discovery service")
				continue
			}
			sort.Slice(newInstances, func(i, j int) bool {
				return newInstances[i] < newInstances[j]
			})
			if !reflect.DeepEqual(currInstances, newInstances) {
				log.WithField("instances", newInstances).Infof("Service %q instances in discovery changed - sending update", serviceName)
				currInstances = newInstances
				instancesChan <- newInstances
			}
		}
	}()

	return instancesChan
}

// DiscoveryServiceClient represents generic discovery service client that can
// return list of services
type DiscoveryServiceClient interface {
	// GetAddrsByName returns list of services with given name
	GetAddrsByName(serviceName string) ([]Address, error)
}

// NewConsulDiscoveryServiceClient returns DiscoverServiceClient backed by Consul
func NewConsulDiscoveryServiceClient(client *api.Client) DiscoveryServiceClient {
	return &consulDiscoveryServiceClient{
		client: client,
	}
}

type consulDiscoveryServiceClient struct {
	client *api.Client
}

func (c *consulDiscoveryServiceClient) GetAddrsByName(serviceName string) ([]Address, error) {
	//TODO(janisz): Add fallback to other datacenters with query
	services, _, err := c.client.Catalog().Service(serviceName, "", nil)

	if err != nil {
		return nil, fmt.Errorf("could NOT find service in Consul: %s", err)
	}

	instances := make([]Address, len(services))
	for i, instance := range services {
		instances[i] = Address(hostPort(instance.ServiceAddress, instance.ServicePort))
	}

	return instances, nil
}

func hostPort(host string, port int) Address {
	return Address(fmt.Sprintf("%s:%d", host, port))
}
