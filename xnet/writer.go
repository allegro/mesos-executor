package xnet

import (
	"fmt"
	"io"
	"net"
	"reflect"
	"sort"
	"strconv"
	"time"

	"github.com/hashicorp/consul/api"
	log "github.com/sirupsen/logrus"
)

// InstanceProvider is the channel where updated list of desired service are published
type InstanceProvider <-chan []Address

// SenderFunc sends payload to given Address
type SenderFunc func(addr Address, payload []byte) (int, error)

// Address of a service in IP:PORT format
type Address string

// RoundRobinWriter returns writer with round robin functionality. Every write could be sent to different backend.
func RoundRobinWriter(instanceProvider InstanceProvider, sender SenderFunc) io.Writer {
	return &roundRobinWriter{provider: instanceProvider, sender: sender, instances: nil}
}

type roundRobinWriter struct {
	provider  InstanceProvider
	sender    SenderFunc
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
}

func (r *roundRobinWriter) write(payload []byte) (int, error) {
	// Read next instance from queue
	instance := <-r.instances
	// Enqueue instance for round robin behaviour
	r.instances <- instance

	return r.sender(instance, payload)
}

// UDPSender returns a SenderFunc that can write payload to the network Address and reuses single connection
func UDPSender() (SenderFunc, error) {
	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, fmt.Errorf("could not create connection: %s", err)
	}

	return func(addr Address, payload []byte) (int, error) {
		udpAddr, err := addressToUDP(addr)
		if err != nil {
			return 0, fmt.Errorf("invalid address %s: %s", addr, err)
		}

		n, err := conn.WriteTo(payload, &udpAddr)
		if err != nil {
			return 0, fmt.Errorf("could not sent payload to %s: %s", addr, err)
		}
		return n, nil
	}, nil
}

func addressToUDP(addr Address) (net.UDPAddr, error) {
	host, p, err := net.SplitHostPort(string(addr))
	if err != nil {
		return net.UDPAddr{}, err
	}

	port, err := strconv.Atoi(p)
	if err != nil {
		return net.UDPAddr{}, err
	}

	return net.UDPAddr{IP: net.ParseIP(host), Port: port}, nil
}

// DiscoveryServiceInstanceProvider returns InstanceProvider that is updated with list of instances in interval
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

// DiscoveryServiceClient represents generic discovery service client that can return list of services
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
