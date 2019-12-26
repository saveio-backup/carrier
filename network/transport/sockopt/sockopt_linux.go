//+build linux

package sockopt

import "syscall"

func SetNonblock(fd uintptr, nonblocking bool) error {
	return syscall.SetNonblock(int(fd), nonblocking)
}
