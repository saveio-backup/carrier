/**
 * Description:
 * Author: LiYong Zhang
 * Create: 2019-01-29
 */
package nat

import (
	"flag"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/gortc/stun"
	"github.com/oniio/oniChain/common/log"
	"github.com/oniio/oniP2p/network"
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
	_          network.ComponentInterface = (*StunComponent)(nil)
	StunServer                            = flag.String("server", fmt.Sprintf(PublicStunSrv1), "Stun server Address")
)

const (
	udp = "udp4"
	rto = 3000
)

func (st *StunComponent) Startup(n *network.Network) {
	log.Infof("Setting up NAT traversal with Stun5389 for address: %s\n", n.Address)
	flag.Parse()
	info, err := network.ParseAddress(n.Address)
	if err != nil {
		log.Fatal("Unable to Parse Address:", err)
	}
	st.internalPort = int(info.Port)
	st.internalIP = net.ParseIP(info.Host)

	srvAddr, err := net.ResolveUDPAddr(udp, *StunServer)
	if err != nil {
		log.Fatal("resolve srvAddr:", err)
	}

	ls := net.JoinHostPort(info.Host, strconv.Itoa(int(info.Port)))
	lAddr, err := net.ResolveUDPAddr(udp, ls)
	conn, err := net.ListenUDP(udp, lAddr)

	st.conn = conn
	if err != nil {
		log.Fatal("listenUDP:", err)
	}

	err = sendBindingRequest(conn, srvAddr)
	var publicAddr stun.XORMappedAddress

	messageChan := listen(conn)
	for {
		message, ok := <-messageChan
		if !ok {
			log.Error("Read from msgCh error")
			break
		}
		if stun.IsMessage(message) {
			m := new(stun.Message)
			m.Raw = message
			err := m.Decode()
			if err != nil {
				log.Warnf("decode:%v", err)
				break
			}
			var xorAddr stun.XORMappedAddress
			if err := xorAddr.GetFrom(m); err != nil {
				break
			}
			if publicAddr.String() != xorAddr.String() {
				log.Infof("My public IP and Port is:%s\n", xorAddr)
				st.externalIP = xorAddr.IP
				st.externalPort = xorAddr.Port

				publicAddr = xorAddr
				break
			}
		} else {
			log.Fatal("unknown message:", message)
		}

	}

	keepAlive := time.Tick(rto * time.Millisecond)
	go func() {
		for {
			select {
			case <-keepAlive:
				err = sendKeepAlive(conn, srvAddr)
				if err != nil {
					log.Fatal("keepalive:", err)
				}
			}

		}
	}()
	n.Nat.Enable = true
	n.Nat.Conn = st.conn
}

/*
func (st *StunComponent) Cleanup(n *network.Network){
	log.Info("Cleanup conn...")
	st.conn.Close()
}
*/

func RegisterStunComponent(builder *network.Builder) {
	builder.AddComponentWithPriority(-99998, new(StunComponent))
}

func (st *StunComponent) GetPublicAddr() string {
	return net.JoinHostPort(st.externalIP.String(), strconv.Itoa(st.externalPort))
}
func (st *StunComponent) GetPrivateAddr() string {
	return net.JoinHostPort(st.internalIP.String(), strconv.Itoa(st.internalPort))

}
