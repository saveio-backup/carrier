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

	"sync"

	"github.com/gortc/stun"
	"github.com/saveio/carrier/network"
	"github.com/saveio/themis/common/log"
)

type StunComponent struct {
	*network.Component
	internalIP net.IP
	externalIP net.IP

	internalPort int
	externalPort int
	mu           sync.Mutex
	conn         *net.UDPConn
}

var (
	StunComponentID                            = (*StunComponent)(nil)
	_               network.ComponentInterface = (*StunComponent)(nil)
	StunServer                                 = flag.String("server", fmt.Sprintf(PublicStunSrv1), "Stun server Address")
	Kill            chan struct{}
)

const (
	udp = "udp4"
	rto = 3000
)

func (st *StunComponent) Startup(n *network.Network) {
	log.Infof("Setting up NAT traversal with Stun5389 for address: %s\n", n.Address)
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

	conn := n.Conn
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
				log.Error(err)
				continue
			}
			if publicAddr.String() != xorAddr.String() {
				st.mu.Lock()
				log.Infof("My public IP and Port is :%s\n", xorAddr)
				st.externalIP = xorAddr.IP
				st.externalPort = xorAddr.Port
				publicAddr = xorAddr
				Kill = make(chan struct{})
				close(Kill)
				st.mu.Unlock()
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
