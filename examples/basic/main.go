/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-08-13
 */
package main

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

func server() {
	listener, err := net.Listen("tcp", "10.1.2.5:"+strconv.Itoa(6109))
	if err != nil {
		fmt.Println("Listen err:", err.Error(), "time:", time.Now())
	} else {
		fmt.Println("Listen addr:", listener.Addr(), "time:", time.Now())
	}
	conn, err := listener.Accept()
	if err != nil {
		fmt.Println("Accept err:", err.Error(), ",time:", time.Now())
	} else {
		fmt.Println("New inbound, remote addr:", conn.RemoteAddr().String(), ",time:", time.Now())
	}
	for {
		buffer := make([]byte, 256)
		bytes, err := conn.Read(buffer)
		if err != nil {
			fmt.Println("conn.Read err:", err.Error(), ",time:", time.Now())
		} else {
			fmt.Println("Read bytes:", bytes, "value:", buffer, ",time:", time.Now())
		}
	}
}

func main() {
	resolved, err := net.ResolveTCPAddr("tcp", "40.73.103.72:6109")
	if err != nil {
	}

	conn, err := net.DialTCP("tcp", nil, resolved)
	if err != nil {
		fmt.Println("conn err:", err.Error())
	} else {
		fmt.Println("conn.addr:", conn.RemoteAddr().String(), ",time:", time.Now())
	}
	buffer := make([]byte, 1024)
	go func() {
		for {
			conn.SetWriteDeadline(time.Now().Add(time.Second * 2))
			_, err := conn.Write([]byte("Helloworld"))
			if err != nil {
				fmt.Println("write err:", err.Error())
			}
			fmt.Println("send success:Helloworld, time:", time.Now().String())
			time.Sleep(3 * time.Second)
		}
	}()
	bytes, err := conn.Read(buffer)
	if err != nil {
		fmt.Println("Read err:", err.Error(), ",time:", time.Now())
	} else {
		fmt.Println("read some bytes,len:", bytes, ",value:", buffer, ",time:", time.Now())
	}
}
