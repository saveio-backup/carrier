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

func KCPComponentRestartUp(n *network.Network) {
	if n.ProxyModeEnable() == false {
		log.Error("please enable kcp proxy firstly.")
		return
	}

	n.ProxyService.Finish.Store("kcp", make(chan struct{}))
	client, _ := n.Client(n.GetWorkingProxyServer())
	if err := client.Tell(context.Background(), &protobuf.ProxyRequest{}); err != nil {
		log.Error("kcp proxy component restart err:", err.Error())
	}
	n.BlockUntilKCPProxyFinish()
}

// Startup implements the Component callback
func KCPComponentStartup(n *network.Network) {
	client, _ := n.Client(n.GetWorkingProxyServer())
	if err := client.Tell(context.Background(), &protobuf.ProxyRequest{}); err != nil {
		log.Error("kcp proxy component start err:", err.Error())
	}
}

func KCPComponentReceive(ctx *network.ComponentContext) error {
	switch ctx.Message().(type) {
	case *protobuf.ProxyResponse:
		log.Info("Node(kcp) public ip is:", ctx.Message().(*protobuf.ProxyResponse).ProxyAddress)
		ctx.Network().ID.Address = "kcp://" + ctx.Message().(*protobuf.ProxyResponse).ProxyAddress
		ctx.Network().FinishProxyServer("kcp")
	}

	return nil
}
