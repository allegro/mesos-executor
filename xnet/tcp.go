package xnet

import (
	"fmt"
	"net"

	log "github.com/sirupsen/logrus"
)

// TCPSender is a Sender implementation that can write payload to the network
// address and reuses TCP connections for the same addresses.
type TCPSender struct {
	Dialer net.Dialer

	connections map[Address]net.Conn
}

// Send sends given payload to passed address. Data is sent using pool of TCP
// connections. It returns number of bytes sent and error - if there was any.
func (s *TCPSender) Send(addr Address, payload net.Buffers) (int, error) {
	if s.connections == nil {
		s.connections = make(map[Address]net.Conn)
	}
	conn, ok := s.connections[addr]
	if !ok {
		newConn, err := s.dial(addr)
		if err != nil {
			return 0, fmt.Errorf("unable to dial %s address: %s", addr, err)
		}
		s.connections[addr] = newConn
		conn = newConn
	}
	n, err := payload.WriteTo(conn)
	if err != nil {
		log.WithError(err).Info("Closing TCP connection because of an error")
		// let's be nice and at least try to close connection on our side
		closeErr := s.connections[addr].Close()
		if closeErr != nil {
			log.WithError(closeErr).Warn("Unable to close TCP connection properly")
		}
		delete(s.connections, addr)
	}
	return int(n), err
}

// Release frees system sockets used by sender.
func (s *TCPSender) Release() error {
	if s.connections == nil {
		return nil
	}
	var errs []error
	for _, conn := range s.connections {
		if err := conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	s.connections = nil
	if len(errs) > 0 {
		return MultiError(errs)
	}
	return nil
}

func (s *TCPSender) dial(addr Address) (net.Conn, error) {
	conn, err := s.Dialer.Dial("tcp", string(addr))
	if err != nil {
		return nil, err // we want plain error here
	}
	return conn, nil
}
