package proxy

import (
	"github.com/saveio/carrier/network"
	"fmt"
)


type ProxyComponent struct {
	*network.Component
}

// Startup implements the Component callback
func (p *ProxyComponent) Startup(n *network.Network) {
	n.Transports().Range(func(protocol, _ interface{})bool{
		if protocol.(string) == "udp"{
			UDPComponentStartup(n)
		}
		if protocol.(string) == "kcp"{
			KCPComponentStartup(n)
		}
		return true
	})
}

func (p *ProxyComponent) Receive(ctx *network.ComponentContext) error {
	ctx.Network().Transports().Range(func(protocol, _ interface{})bool{
		if protocol.(string) == "udp" {
			UDPComponentReceive(ctx)
		}
		if protocol.(string) == "kcp"{
			fmt.Println("proxy component receive, protocol is kcp...")
			KCPComponentReceive(ctx)
		}
		return true
	})
	return nil
}
