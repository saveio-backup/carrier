/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-07-10
*/
package common

import (
	"net"
	"time"
)

type ProxiedConn struct {
	Conn       		net.Conn
	RemoteAddress 	*ProxiedAddr
	BoundAddr  		*ProxiedAddr
}

func (c *ProxiedConn) Read(b []byte) (int, error) {
	return c.Conn.Read(b)
}

func (c *ProxiedConn) Write(b []byte) (int, error) {
	return c.Conn.Write(b)
}

func (c *ProxiedConn) Close() error {
	return c.Conn.Close()
}

func (c *ProxiedConn) LocalAddr() net.Addr {
	if c.BoundAddr != nil {
		return c.BoundAddr
	}
	return c.Conn.LocalAddr()
}

func (c *ProxiedConn) RemoteAddr() net.Addr {
	if c.RemoteAddress != nil {
		return c.RemoteAddress
	}
	return c.Conn.RemoteAddr()
}

func (c *ProxiedConn) SetDeadline(t time.Time) error {
	return c.Conn.SetDeadline(t)
}

func (c *ProxiedConn) SetReadDeadline(t time.Time) error {
	return c.Conn.SetReadDeadline(t)
}

func (c *ProxiedConn) SetWriteDeadline(t time.Time) error {
	return c.Conn.SetWriteDeadline(t)
}
