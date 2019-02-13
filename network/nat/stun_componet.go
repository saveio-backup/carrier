/**
 * Description:
 * Author: LiYong Zhang
 * Create: 2019-01-29
 */
package nat

import (
	"flag"
	"fmt"
	"github.com/golang/glog"
	"github.com/gortc/stun"
	"github.com/oniio/oniP2p/network"
	"net"
	"time"
	"strconv"
)

type StunComponent struct {
	*network.Component

	internalIP net.IP
	externalIP net.IP

	internalPort int
	externalPort int

	conn *net.UDPConn
}

var (
	// StunComponentID to reference NAT Component
	StunComponentID                            = (*StunComponent)(nil)
	_               network.ComponentInterface = (*StunComponent)(nil)
	StunServer                                 = flag.String("server", fmt.Sprintf("stun.l.google.com:19302"), "Stun server Address")
)

const (
	udp     = "udp4"
	rto     = 3000
)

func (st *StunComponent) Startup(n *network.Network) {
	glog.Infof("Setting up NAT traversal with Stun5389 for address: %s", n.Address)
	flag.Parse()
	info, err := network.ParseAddress(n.Address)
	if err != nil {
		glog.Fatalln("Unable to Parse Address: ", err)
	}
	st.internalPort = int(info.Port)
	st.internalIP = net.ParseIP(info.Host)

	srvAddr, err := net.ResolveUDPAddr(udp, *StunServer)
	if err != nil {
		glog.Fatalln("resolve srvAddr:", err)
	}

	ls:=net.JoinHostPort(info.Host,strconv.Itoa(int(info.Port)))
	lAddr,err:=net.ResolveUDPAddr(udp,ls)
	conn, err := net.ListenUDP(udp, lAddr)

	st.conn=conn
	if err != nil {
		glog.Fatalln("listenUDP:", err)
	}

	glog.Infof("Listening on %s\n", conn.LocalAddr())
	err = sendBindingRequest(conn,srvAddr)
	var publicAddr stun.XORMappedAddress

	messageChan := listen(conn)
	for {
		message,ok :=<-messageChan
		if !ok{
			glog.Error("Read from msgCh error")
			break
		}
		if stun.IsMessage(message){
			m := new(stun.Message)
			m.Raw = message
			err := m.Decode()
			if err != nil {
				glog.Warningln("decode:",err)
				break
			}
			var xorAddr stun.XORMappedAddress
			if err := xorAddr.GetFrom(m); err != nil{
				break
			}
			if publicAddr.String() != xorAddr.String(){
				glog.Infof("My public IP and Port is:%s",xorAddr)
				st.externalIP=xorAddr.IP
				st.externalPort=xorAddr.Port

				publicAddr = xorAddr
				break
			}
		}else{
			glog.Fatalln("unknown message", message)
		}

	}

	keepAlive:=time.Tick(rto * time.Millisecond)
	go func() {
		for {
			select {
			case <-keepAlive:
				err = sendKeepAlive(conn,srvAddr)
				if err!=nil {
					glog.Fatalln("keepalive:", err)
				}
			}

		}
	}()
}
/*
func (st *StunComponent) Cleanup(n *network.Network){
	glog.Infoln("Cleanup conn...")
	st.conn.Close()
}
*/

func RegisterStunComponent(builder *network.Builder) {
	builder.AddComponentWithPriority(-99998, new(StunComponent))
}

func(st *StunComponent)GetPublicAddr()string{
	return net.JoinHostPort(st.externalIP.String(),strconv.Itoa(st.externalPort))
}
func (st *StunComponent)GetPrivateAddr()string  {
	return net.JoinHostPort(st.internalIP.String(),strconv.Itoa(st.internalPort))

}



