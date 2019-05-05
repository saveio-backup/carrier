package proxy

import (
	"context"
	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/carrier/network"
	"time"
	"github.com/saveio/themis/common/log"
)

type ProxyComponent struct {
	*network.Component
}

// Startup implements the Component callback
func (p *ProxyComponent) Startup(net *network.Network) {
	go func() {
		for{
			net.Bootstrap(net.GetProxyServer())
			client := net.GetPeerClient(net.GetProxyServer())
			if client == nil{
				log.Info("Waiting one minute for Dialing to Proxy-Server successed...")
				time.Sleep(time.Second*1)
			}else{
				client.Tell(context.Background(),&protobuf.ProxyRequest{})
				return
			}
		}
	}()
}

func (p *ProxyComponent) Receive(ctx *network.ComponentContext) error {
	switch ctx.Message().(type) {
	case *protobuf.ProxyResponse:
		log.Info("Node public ip is:", ctx.Message().(*protobuf.ProxyResponse).ProxyAddress)
		ctx.Network().ID.Address= "udp://"+ctx.Message().(*protobuf.ProxyResponse).ProxyAddress
	}

	return nil
}
