package xnet

import (
	"fmt"
	"net"
)

// UDPSender is a Sender implementation that can write payload to the network
// address and reuses single system socket. It uses UDP packets to send data.
type UDPSender struct {
	conn *net.UDPConn
}

// Send sends given payload to passed address. Data is sent using UDP packets.
// It returns number of bytes sent and error - if there was any.
func (s *UDPSender) Send(addr Address, buffer net.Buffers) (int, error) {
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

	var total int
	for _, b := range buffer {
		n, err := s.conn.WriteTo(b, udpAddr)
		total += n
		if err != nil {
			return total, fmt.Errorf("could not sent payload to %s: %s", addr, err)
		}
	}

	return total, nil
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
