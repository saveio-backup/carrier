/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-01-07
 */

package transport

import (
	"net"
	"time"
)

// UDP represents the UDP transport protocol alongside its respective configurable options.
type UDP struct {
	WriteBufferSize int
	ReadBufferSize  int
	NoDelay         bool
}

// NewUDP instantiates a new instance of the UDP transport protocol.
func NewUDP() *UDP {
	return &UDP{
		WriteBufferSize: 10000,
		ReadBufferSize:  10000,
		NoDelay:         false,
	}
}

// Listen listens for incoming UDP connections on a specified port.
func (t *UDP) Listen(address string) (interface{}, error) {
	resolved, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}
	listener, err := net.ListenUDP("udp", resolved)
	if err != nil {
		return nil, err
	}
	return interface{}(listener), nil
}

// Dial dials an address via. the UDP protocol.
func (t *UDP) Dial(address string, timeout time.Duration) (interface{}, error) {
	resolved, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP("udp", nil, resolved)
	if err != nil {
		return nil, err
	}

	return interface{}(conn), nil
}
