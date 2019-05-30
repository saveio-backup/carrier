/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-05-15
 */
package proxy

import (
	"context"
	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/carrier/network"
	"github.com/saveio/themis/common/log"
)

// Startup implements the Component callback
func KCPComponentStartup(n *network.Network) {
	client, _ := n.Client(n.GetProxyServer())
	client.Tell(context.Background(), &protobuf.ProxyRequest{})
}

func KCPComponentReceive(ctx *network.ComponentContext) error {
	switch ctx.Message().(type) {
	case *protobuf.ProxyResponse:
		log.Info("Node public ip is:", ctx.Message().(*protobuf.ProxyResponse).ProxyAddress)

		relayIP := "kcp://" + ctx.Message().(*protobuf.ProxyResponse).ProxyAddress

		if relayIP == ctx.Network().ID.Address {
			ctx.Network().DeletePeerClient(ctx.Network().GetProxyServer())
		} else {
			ctx.Network().ID.Address = relayIP
		}
		ctx.Network().FinishProxyServer("kcp")
	}

	return nil
}
