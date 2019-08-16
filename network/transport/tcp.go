package transport

import (
	"net"
	"strconv"
	"strings"
	"time"
)

// TCP represents the TCP transport protocol alongside its respective configurable options.
type TCP struct {
	WriteBufferSize int
	ReadBufferSize  int
	NoDelay         bool
}

// NewTCP instantiates a new instance of the TCP transport protocol.
func NewTCP() *TCP {
	return &TCP{
		WriteBufferSize: 10000,
		ReadBufferSize:  10000,
		NoDelay:         false,
	}
}

func resolveTcpAddr(address string) string {
	if strings.HasPrefix(address, "tcp://") {
		return address[len("tcp://"):]
	}
	return address
}

// Listen listens for incoming TCP connections on a specified port.
func (t *TCP) Listen(port int) (interface{}, error) {
	listener, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return nil, err
	}

	return interface{}(listener), nil
}

// Dial dials an address via. the TCP protocol.
func (t *TCP) Dial(address string, timeout time.Duration) (interface{}, error) {
	conn, err := net.DialTimeout("tcp", resolveTcpAddr(address), timeout)
	if err != nil {
		return nil, err
	}
	return interface{}(conn), nil
}
