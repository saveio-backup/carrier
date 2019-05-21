/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-05-21 
*/
package main

import (
"flag"
"fmt"
"net"
"syscall"
)

var (
	ttl        int
	daemon     bool
	port       = 12389
	multiaddr  = [4]byte{224, 0, 1, 1}
	inaddr_any = [4]byte{0, 0, 0, 0}
)

func init() {
	multi := flag.String("m", "224.0.1.1", "-m 224.0.1.1 specify multicast address")
	flag.IntVar(&port, "p", 12389, "-p 12389 specify multi address listen port")
	flag.IntVar(&ttl, "t", 8, "-t 8 specify ttl value")
	flag.BoolVar(&daemon, "d", false, "-d is a recv client")
	flag.Parse()
	ip := net.ParseIP(*multi)
	if ip != nil {
		copy(multiaddr[:], ip[12:16])
		if CheckMultiCast(multiaddr) {
			return
		}
	}
	fmt.Println("Isvalid multi address")
	syscall.Exit(1)
}

func main() {
	socketMC, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		fmt.Printf("Create socket fd error:%s\n", err.Error())
		return
	}
	defer syscall.Close(socketMC)
	if err = SetTTL(socketMC, ttl); err != nil {
		fmt.Printf("Set ttl error:%s\n", err.Error())
		return
	}

	if daemon {
		err = UDPMulticast(socketMC)
		if err == nil {
			RecvMsg(socketMC)
		} else {
			fmt.Printf("Join multicast error:%s\n", err.Error())
		}
	} else {
		SendMsg(socketMC)
	}
}

func SendMsg(socketMC int) {
	var (
		msg   = []byte("Hello world")
		raddr = &syscall.SockaddrInet4{Port: port, Addr: multiaddr}
	)

	if err := syscall.Sendto(socketMC, msg, 0, raddr); err != nil {
		fmt.Printf("Send msg error:%s\n", err.Error())
	}
}

func RecvMsg(socketMC int) {
	var buf = make([]byte, 256)
	for {
		n, p, e := syscall.Recvfrom(socketMC, buf, 0)
		if e != nil {
			break
		}
		fmt.Printf("Recv:%s\n", string(buf[:n]))
		//raddr, ok := p.(syscall.SockaddrInet4)
		raddr, ok := p.(syscall.SockaddrInet4)
		if ok {
			fmt.Printf("Remote addr:%d.%d.%d.%d:%d\n", raddr.Addr[0], raddr.Addr[1], raddr.Addr[2], raddr.Addr[3], raddr.Port)
		} else {
			fmt.Printf("Remote info:%v\n", p)
		}
	}
	ExitMultiCast(socketMC)
}

//加入组播域
func UDPMulticast(socketMC int) error {
	err := syscall.Bind(socketMC, &syscall.SockaddrInet4{Port: port, Addr: inaddr_any})
	if err == nil {
		var mreq = &syscall.IPMreq{Multiaddr: multiaddr, Interface: inaddr_any}
		err = syscall.SetsockoptIPMreq(socketMC, syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, mreq)
	}
	return err
}

//退出组播域
func ExitMultiCast(socketMC int) {
	var mreq = &syscall.IPMreq{Multiaddr: multiaddr, Interface: inaddr_any}
	syscall.SetsockoptIPMreq(socketMC, syscall.IPPROTO_IP, syscall.IP_DROP_MEMBERSHIP, mreq)
}

//设置路由的TTL值
func SetTTL(fd, ttl int) error {
	return syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_MULTICAST_TTL, ttl)
}

//检查是否是有效的组播地址范围
func CheckMultiCast(addr [4]byte) bool {
	if addr[0] > 239 || addr[0] < 224 {
		return false
	}
	if addr[2] == 0 {
		return addr[3] <= 18
	}
	return true
}
