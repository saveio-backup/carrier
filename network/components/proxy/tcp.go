/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-05-31
 */
package proxy

import (
	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/carrier/network"
	"github.com/saveio/themis/common/log"
)

// Startup implements the Component callback
func TcpComponentRestartUp(n *network.Network) {
	if n.ProxyModeEnable() == false {
		log.Error("please enable tcp proxy firstly.")
		return
	}

	n.ProxyService.Finish.Store("tcp", make(chan struct{}))

	var i int
	for i = 0; i < len(n.ProxyService.Servers); i++ {
		if err := n.ConnectProxyServer(n.GetWorkingProxyServer()); err != nil {
			log.Error("tcp proxy component start err:", err.Error(), "proxy server:", n.GetWorkingProxyServer(), "proxy workID:", n.ProxyService.WorkID)
			n.UpdateProxyWorkID()
			continue
		} else {
			n.BlockUntilTcpProxyFinish()
			log.Info("successed restart and connect to proxy server:", n.GetWorkingProxyServer(), "proxy server workID:", n.ProxyService.WorkID)
			return
		}
	}
	log.Error("in ComponentRestartUp, all backup proxy server IP has been tried again, failed to re-connect to proxy server.")
}

// Startup implements the Component callback
func TcpComponentStartup(n *network.Network) {
	if n.ProxyModeEnable() == false {
		log.Error("please enable tcp proxy firstly.")
		return
	}

	for i := 0; i < len(n.ProxyService.Servers); i++ {
		if err := n.ConnectProxyServer(n.GetWorkingProxyServer()); err != nil {
			log.Error("tcp proxy component start err:", err.Error(), "proxy server:", n.GetWorkingProxyServer(), "proxy workID:", n.ProxyService.WorkID)
			n.UpdateProxyWorkID()
			continue
		} else {
			log.Info("successed about dialing to proxy server:", n.GetWorkingProxyServer(), "proxy server workID:", n.ProxyService.WorkID)
			return
		}
	}
	log.Error("in ComponentStartUp, all backup proxy server IP has been tried again, failed to connect to proxy server.")
}

func TcpComponentReceive(ctx *network.ComponentContext) error {
	switch ctx.Message().(type) {
	case *protobuf.ProxyResponse:
		log.Info("Node(tcp) public ip is:", ctx.Message().(*protobuf.ProxyResponse).ProxyAddress)
		ctx.Network().ID.Address = "tcp://" + ctx.Message().(*protobuf.ProxyResponse).ProxyAddress
		ctx.Network().FinishProxyServer("tcp")
	}

	return nil
}
