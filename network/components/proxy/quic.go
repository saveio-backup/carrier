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

func QuicComponentRestartUp(n *network.Network) {
	if n.ProxyModeEnable() == false {
		log.Error("please enable quic proxy firstly.")
		return
	}

	n.ProxyService.Finish.Store("quic", make(chan struct{}))
	client, err := n.Client(n.GetWorkingProxyServer())
	if err != nil {
		log.Error("new client err in quic component restart, err:", err.Error())
		return
	}
	client.Tell(context.Background(), &protobuf.ProxyRequest{})

	n.BlockUntilQuicProxyFinish()
}

// Startup implements the Component callback
func QuicComponentStartup(n *network.Network) {
	client, err := n.Client(n.GetWorkingProxyServer())
	if err != nil {
		log.Error("new client err in quic component startup, err:", err.Error())
		return
	}
	client.Tell(context.Background(), &protobuf.ProxyRequest{})
}

func QuicComponentReceive(ctx *network.ComponentContext) error {
	switch ctx.Message().(type) {
	case *protobuf.ProxyResponse:
		log.Info("Node(quic) public ip is:", ctx.Message().(*protobuf.ProxyResponse).ProxyAddress)
		ctx.Network().ID.Address = "quic://" + ctx.Message().(*protobuf.ProxyResponse).ProxyAddress
		ctx.Network().FinishProxyServer("quic")
	}

	return nil
}
