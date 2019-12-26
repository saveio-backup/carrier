//+build windows

package sockopt

import "syscall"

func SetNonblock(fd uintptr, nonblocking bool) error {
	return syscall.SetNonblock(syscall.Handle(fd), nonblocking)
}
