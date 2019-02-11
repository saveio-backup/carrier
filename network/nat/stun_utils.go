/**
 * Description:
 * Author: LiYong Zhang
 * Create: 2019-01-30 
*/
package nat

import (
	"fmt"
	"bufio"
	"os"
	"github.com/golang/glog"
	"github.com/gortc/stun"
	"strings"
	"net"
)

func listen(conn *net.UDPConn) <-chan []byte {
	messages := make(chan []byte)
	go func() {
		for {
			buf := make([]byte, 1024)

			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				close(messages)
				return
			}
			buf = buf[:n]
			messages <- buf
		}
	}()
	return messages
}

func getPeerAddr() <-chan string {
	result := make(chan string)

	go func() {
		reader := bufio.NewReader(os.Stdin) //FIXME: change scan method
		glog.Infoln("Enter remote peer address:")
		peer, _:=reader.ReadString('\n')
		result <- strings.Trim(peer,"\r\n")
	}()

	//glog.Infoln("get peer result:",<-result) //FIXME: TO TEST
	return result
}

func sendBindingRequest(conn *net.UDPConn, addr *net.UDPAddr) error{
	m:=stun.MustBuild(stun.TransactionID,stun.BindingRequest)

	err:= send(m.Raw,conn,addr)
	if err!=nil{
		return fmt.Errorf("bindingReq: %v",err)
	}
	return nil
}

func send(msg []byte, conn *net.UDPConn, addr *net.UDPAddr) error{
	_, err := conn.WriteToUDP(msg, addr)
	if err !=nil {
		return fmt.Errorf("sendBindReq: %v",err)
	}
	return nil
}

func sendStr(msg string, conn *net.UDPConn, addr *net.UDPAddr)error{
	glog.Infoln("msgstr:",msg)
	return send([]byte(msg), conn, addr)
}

func GetValidLocalIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err!=nil{
		glog.Fatalln(err)
	}
	defer conn.Close()
	//localAddr := conn.LocalAddr().String()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	//idx := strings.LastIndex(localAddr, ":")
	fmt.Printf("local addr:%s\n",localAddr)
	//return localAddr[0:idx]
	return localAddr.IP

}
