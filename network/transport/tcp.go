package transport

import (
	"context"
	"net"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/saveio/carrier/network/transport/sockopt"
	"github.com/saveio/themis/common/log"
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
	var lc net.ListenConfig
	lc.Control = func(network, address string, c syscall.RawConn) error {
		return c.Control(func(fd uintptr) {
			if err := sockopt.SetNonblock(fd, true); err != nil {
				log.Errorf("in tcp listen err when set non-block,err:%s", err.Error())
			} else {
				log.Info("set non-block success when listen, value is true")
			}
		})
	}
	listener, err := lc.Listen(context.Background(), "tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return nil, err
	}
	return interface{}(listener), nil
}

// Dial dials an address via. the TCP protocol.
func (t *TCP) Dial(address string, timeout time.Duration) (interface{}, error) {
	dialer := &net.Dialer{
		Timeout:   timeout,
		DualStack: true,
		LocalAddr: nil,
	}
	dialer.Control = func(network, address string, c syscall.RawConn) error {
		return c.Control(func(fd uintptr) {
			if err := sockopt.SetNonblock(fd, true); err != nil {
				log.Errorf("in tcp dial err when set non-block,err:%s", err.Error())
			} else {
				log.Info("se non-block success when dial, value is true")
			}
		})
	}
	conn, err := dialer.DialContext(context.Background(), "tcp", resolveTcpAddr(address))
	if err != nil {
		return nil, err
	}
	return interface{}(conn), nil
}
