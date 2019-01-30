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
	pingMsg = "ping"
	pongMsg = "pong"
	rto     = 500
)

func (st *StunComponent) Startup(n *network.Network) {
	glog.Infof("Setting up NAT traversal with Stun5389 for address: %s", n.Address)
	flag.Parse()
	info, err := network.ParseAddress(n.Address)
	if err != nil {
		glog.Warning("Unable to Parse Address: ", err)
		return
	}
	st.internalPort = int(info.Port)

	srvAddr, err := net.ResolveUDPAddr(udp, *StunServer)
	if err != nil {
		glog.Fatalln("resolve srvAddr:", err)
	}
	conn, err := net.ListenUDP(udp, nil)
	st.conn=conn
	if err != nil {
		glog.Fatalln("listenUDP:", err)
	}

	defer st.Cleanup(n)  //FIXME? use network.conn?

	glog.Infof("Listening on %s\n", conn.LocalAddr())

	var publicAddr stun.XORMappedAddress
	var peerAddr *net.UDPAddr

	messageChan := listen(conn)
	var peerAddrChan <-chan string

	keepAlive := time.Tick(rto * time.Millisecond)
	keepAliveMsg := pingMsg

	var quit <-chan time.Time

	gotPong := false
	sentPong := false

	for {
		select {
		case message, ok := <-messageChan:
			if !ok {
				glog.Infoln("no message in messageChan") //FIXME: FOR TEST
				return
			}
			switch {
			case string(message) == pingMsg:
				keepAliveMsg = pongMsg
			case string(message) == pongMsg:
				if !gotPong {
					glog.Infoln("Received pong message.")
				}
				gotPong = true

				keepAliveMsg = pongMsg

			case stun.IsMessage(message):
				m := new(stun.Message)
				m.Raw = message
				err := m.Decode()
				if err != nil {
					glog.Warningln("decode:",err)
					break
				}
				var xorAddr stun.XORMappedAddress
				if err := xorAddr.GetFrom(m); err!=nil{
					glog.Infoln("getFrom:",err)
					break
				}

				if publicAddr.String() != xorAddr.String(){
					glog.Infof("My public IP and Port is:%s",xorAddr)

					st.externalIP=xorAddr.IP
					st.externalPort=xorAddr.Port

					publicAddr = xorAddr

					peerAddrChan = getPeerAddr()
				}

			default:
				glog.Fatalln("unknown message", message)

			}

			case peerStr := <-peerAddrChan:
				peerAddr ,err = net.ResolveUDPAddr(udp,peerStr)
				glog.Infof("peerAddr :%s",peerAddr) //FIXME: for testing
				if err!=nil{
					glog.Info("resolve peeraddr:",err)
				}

		case <-keepAlive:
			//keep alive with stun server or the peer which he has knew
			if peerAddr == nil{
				err = sendBindingRequest(conn,srvAddr)
			} else {
				err = sendStr(keepAliveMsg, conn, peerAddr)
				if keepAliveMsg == pongMsg{
					sentPong = true
				}
			}
			if err!=nil {
				glog.Fatalln("keepalive:", err)
			}

			case <-quit:
				st.Cleanup(n) //FIXME? make clean conn with stun server only!
		}

		if quit == nil && gotPong && sentPong{
			glog.Infoln("Success! Quiting in two seconds. ")
			quit = time.After(2 * time.Second)
		}
	}

}

func (st *StunComponent) Cleanup(n *network.Network){
	glog.Infoln("Cleanup conn...")
	st.conn.Close()
}

func RegisterStunComponent(builder *network.Builder) {
	builder.AddComponentWithPriority(-99998, new(StunComponent))
}

func(st *StunComponent)GetPublicAddr()string{
	return net.JoinHostPort(st.externalIP.String(),strconv.Itoa(st.externalPort))
}
