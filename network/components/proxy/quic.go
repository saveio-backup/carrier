/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-05-31 
*/
package proxy

import (
	"context"
	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/carrier/network"
	"github.com/saveio/themis/common/log"
)

// Startup implements the Component callback
func QuicComponentStartup(n *network.Network) {
	client, err := n.Client(n.GetProxyServer())
	if err!=nil{
		log.Error("new client err in quic component startup, err:", err.Error())
		return
	}
	client.Tell(context.Background(), &protobuf.ProxyRequest{})
}

func QuicComponentReceive(ctx *network.ComponentContext) error {
	switch ctx.Message().(type) {
	case *protobuf.ProxyResponse:
		log.Info("Node public ip is:", ctx.Message().(*protobuf.ProxyResponse).ProxyAddress)

		relayIP := "quic://" + ctx.Message().(*protobuf.ProxyResponse).ProxyAddress

		if relayIP == ctx.Network().ID.Address {
			ctx.Network().DeletePeerClient(ctx.Network().GetProxyServer())
		} else {
			ctx.Network().ID.Address = relayIP
		}
		ctx.Network().FinishProxyServer("quic")
	}

	return nil
}
