/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-01-07
 */

package transport

import (
	"fmt"
	"net"
	"strconv"
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
func (t *UDP) Listen(port int) (interface{}, error) {
	resolved, err := net.ResolveUDPAddr("udp", ":"+strconv.Itoa(port))
	if err != nil {
		return nil, err
	}
	listener, err := net.ListenUDP("udp", resolved)
	if err != nil {
		return nil, err
	}
	fmt.Printf("UDP listen-handle-ptr:%v\n", listener)
	return interface{}(listener), nil
}

// Dial dials an address via. the UDP protocol.
func (t *UDP) Dial(address string) (interface{}, error) {
	resolved, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP("udp", nil, resolved)
	if err != nil {
		return nil, err
	}

	conn.SetWriteBuffer(t.WriteBufferSize)
	conn.SetReadBuffer(t.ReadBufferSize)
	fmt.Printf("\nDial.address-ptr:%v,remote-address-IP:%s, local-address-IP:%s\n", conn, conn.RemoteAddr(), conn.LocalAddr())
	return interface{}(conn), nil
}
