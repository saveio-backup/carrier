//+build linux

package sockopt

import "syscall"

func SetNonblock(fd uintptr, nonblocking bool) error {
	return syscall.SetNonblock(int(fd), nonblocking)
}

func SetSocksAddrReusedImmediately(fd uintptr, value int) error {
	return syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.SO_REUSEADDR, value)
}

func SetSocksPortReusedImmediately(fd uintptr, value int) error {
	return syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.SO_REUSEPORT, value)
}
