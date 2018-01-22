package xnettest

import (
	"net"
)

// LoopbackServer creates a new network Listener that is binded to the loopback
// interface and can be used to test tcp/udp connections. Listener must be
// closed at the end of the tests to release system resources. It returns
// configured listener and the channel to which it will send received data.
func LoopbackServer(network string) (net.Listener, <-chan []byte, error) {
	listener, err := net.Listen(network, "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	results := make(chan []byte)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // if we are unable to accept connections listener is probably closed
			}
			go func() {
				for {
					buf := make([]byte, 1024)
					n, err := conn.Read(buf)
					if err != nil {
						_ = conn.Close()
						return
					}
					results <- buf[0:n]
				}
			}()
		}
	}()
	return listener, results, nil
}
