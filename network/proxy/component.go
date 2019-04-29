package proxy

import (
	"context"
	"github.com/oniio/oniP2p/internal/protobuf"
	"github.com/oniio/oniP2p/network"
	"time"
	"github.com/oniio/oniChain/common/log"
)

const PROXY_SERVE  = "udp://127.0.0.1:6008"

type ProxyComponent struct {
	*network.Component
}

// Startup implements the Component callback
func (p *ProxyComponent) Startup(net *network.Network) {
	go func() {
		for{
			client := net.GetPeerClient(PROXY_SERVE)
			if client == nil{
				log.Info("Waiting one minute for Dialing to Proxy-Server successed...")
				time.Sleep(time.Second*1)
			}else{
				client.Tell(context.Background(),&protobuf.Proxy{})
				return
			}
		}
	}()
}

func (p *ProxyComponent) Receive(ctx *network.ComponentContext) error {
	switch ctx.Message().(type) {
	case *protobuf.Proxy:
		log.Info("Node public ip is:", ctx.Message().(*protobuf.Proxy).ProxyAddress)
		ctx.Network().ID.Address= "udp://"+ctx.Message().(*protobuf.Proxy).ProxyAddress
	}

	return nil
}
