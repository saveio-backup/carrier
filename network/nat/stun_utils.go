/**
 * Description:
 * Author: LiYong Zhang
 * Create: 2019-01-30 
*/
package nat

import (
	"fmt"
	"github.com/golang/glog"
	"github.com/gortc/stun"
	"net"
	"time"
)
var BindingIndicate =stun.NewType(stun.MethodBinding,stun.ClassIndication)

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


func sendBindingRequest(conn *net.UDPConn, addr *net.UDPAddr) error{
	m:=stun.MustBuild(stun.TransactionID,stun.BindingRequest)

	err:= send(m.Raw,conn,addr)
	if err!=nil{
		return fmt.Errorf("bindingReq: %v",err)
	}
	return nil
}
func sendKeepAlive(conn *net.UDPConn, addr *net.UDPAddr)error{
	m:=stun.MustBuild(stun.TransactionID,BindingIndicate)
	err:= send(m.Raw,conn,addr)
	if err!=nil{
		return fmt.Errorf("sendKeepAlive: %v",err)
	}
	return nil
}

func send(msg []byte, conn *net.UDPConn, addr *net.UDPAddr) error{
	_, err := conn.WriteToUDP(msg, addr)
	if err !=nil {
		return fmt.Errorf("send: %v",err)
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
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP

}

func (st *StunComponent) GetExternalAddr() (ip net.IP,port int) {
	c, err := stun.Dial("udp", "stun.l.google.com:19302")

	if err != nil {
		panic(err)
	}


	message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)

	if err := c.Do(message, func(res stun.Event) {
		if res.Error != nil {
			panic(res.Error)
		}

		var xorAddr stun.XORMappedAddress
		if err := xorAddr.GetFrom(res.Message); err != nil {
			panic(err)
		}

		ip = xorAddr.IP
		port = xorAddr.Port
		st.externalIP=ip
		st.externalPort=port
	}); err != nil {
		panic(err)
	}

	return
}

func (st *StunComponent)keepAlive(srvAddr *net.UDPAddr){
	keepAlive:=time.Tick(rto * time.Millisecond)
	for {

		select {
		case <-keepAlive:
			err := sendBindingRequest(st.conn,srvAddr)
			if err !=nil {
				glog.Fatalln("keepAlive error: ",err)
				break
			}

		}
	}


}