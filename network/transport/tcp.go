package transport

import (
	"context"
	"net"
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

func (t *TCP) Listen(address string) (interface{}, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	return interface{}(listener), nil
}

// Listen listens for incoming TCP connections on a specified port.
func (t *TCP) _Listen(address string) (interface{}, error) {
	var lc net.ListenConfig
	lc.Control = func(network, address string, c syscall.RawConn) error {
		return c.Control(func(fd uintptr) {
			if err := sockopt.SetNonblock(fd, true); err != nil {
				log.Errorf("in tcp listen err when set non-block,err:%s", err.Error())
			} else {
				log.Info("set non-block success when listen, value is true")
			}
			if err := sockopt.SetSocksAddrReusedImmediately(fd, 1); err != nil {
				log.Errorf("in tcp listen err when set addr/port reuse immediately, err:%s", err.Error())
			} else {
				log.Info("set addr/port reuse immediately success when listen, value is true")
			}
		})
	}
	listener, err := lc.Listen(context.Background(), "tcp", address)
	if err != nil {
		return nil, err
	}
	return interface{}(listener), nil
}

func (t *TCP) Dial(address string, timeout time.Duration) (interface{}, error) {
	conn, err := net.DialTimeout("tcp", resolveTcpAddr(address), timeout)
	if err != nil {
		return nil, err
	}
	return interface{}(conn), nil
}

// Dial dials an address via. the TCP protocol.
func (t *TCP) _Dial(address string, timeout time.Duration) (interface{}, error) {
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
				log.Info("set non-block success when dial, value is true")
			}

			if err := sockopt.SetSocksAddrReusedImmediately(fd, 1); err != nil {
				log.Errorf("in tcp dial err when set addr/port reuse immediately, err:%s", err.Error())
			} else {
				log.Info("set addr/port reuse immediately success when dial, value is true")
			}
		})
	}
	conn, err := dialer.DialContext(context.Background(), "tcp", resolveTcpAddr(address))
	if err != nil {
		return nil, err
	}
	return interface{}(conn), nil
}
