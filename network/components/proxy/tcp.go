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
func TcpComponentStartup(n *network.Network) {
	client, err := n.Client(n.GetProxyServer())
	if err != nil {
		log.Error("new client err in tcp component startup, err:", err.Error())
		return
	}
	client.Tell(context.Background(), &protobuf.ProxyRequest{})
}

func TcpComponentReceive(ctx *network.ComponentContext) error {
	switch ctx.Message().(type) {
	case *protobuf.ProxyResponse:
		log.Info("Node(tcp) public ip is:", ctx.Message().(*protobuf.ProxyResponse).ProxyAddress)

		relayIP := "tcp://" + ctx.Message().(*protobuf.ProxyResponse).ProxyAddress

		if relayIP == ctx.Network().ID.Address {
			ctx.Network().DeletePeerClient(ctx.Network().GetProxyServer())
		} else {
			ctx.Network().ID.Address = relayIP
		}
		ctx.Network().FinishProxyServer("tcp")
	}

	return nil
}
