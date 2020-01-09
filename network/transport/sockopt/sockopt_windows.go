//+build windows

package sockopt

import "syscall"

func SetNonblock(fd uintptr, nonblocking bool) error {
	return syscall.SetNonblock(syscall.Handle(fd), nonblocking)
}

func SetSocksAddrReusedImmediately(fd uintptr, value int) error {
	return syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_TCP, syscall.SO_REUSEADDR, value)
}

// some linux version maybe does not have this option, suggest using SO_REUSEADDR
func SetSocksPortReusedImmediately(fd uintptr, value int) error {
	//return syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_TCP, syscall.SO_REUSEPORT, value)
	return nil
}
