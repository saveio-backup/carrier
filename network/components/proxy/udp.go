/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-05-15
 */
package proxy

import (
	"context"
	"net"

	"github.com/golang/protobuf/proto"
	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/carrier/network"
	"github.com/saveio/themis/common/log"
)

func prepareUDPRawMessage(n *network.Network, message proto.Message) ([]byte, *net.UDPAddr, error) {
	signed, err := n.PrepareMessage(context.Background(), message)
	if err != nil {
		log.Error("failed to sign message in send proxy request")
		return nil, nil, err
	}

	buffer := n.BuildRawContent(signed)
	proxySrv, _ := n.GetWorkingProxyServer()
	addrInfo, err := network.ParseAddress(proxySrv)
	if err != nil {
		log.Error("parse address error in send proxy request:", err.Error())
		return nil, nil, err
	}
	resolved, err := net.ResolveUDPAddr("udp", addrInfo.HostPort())
	if err != nil {
		return nil, nil, err
	}
	return buffer, resolved, nil
}

func sendUDPProxyRequest(n *network.Network) error {
	buffer, resolved, err := prepareUDPRawMessage(n, &protobuf.ProxyRequest{})
	if err != nil {
		return err
	}
	_, err = n.Conn.WriteToUDP(buffer, resolved)
	if err != nil {
		log.Error("write error in proxy-component:", err.Error())
		return err
	}
	return nil
}

// Startup implements the Component callback
func UDPComponentRestartUp(n *network.Network) {
	if n.ProxyModeEnable() == false {
		log.Error("please enable udp proxy firstly.")
		return
	}
	n.ProxyService.Finish.Store("udp", make(chan struct{}))

	if err := sendUDPProxyRequest(n); err != nil {
		log.Error("udp proxy compnent restart error:", err.Error())
	}

	n.BlockUntilUDPProxyFinish()
}

// Startup implements the Component callback
func UDPComponentStartup(n *network.Network) {
	if err := sendUDPProxyRequest(n); err != nil {
		log.Error("udp proxy compnent start-up error:", err.Error())
	}
}

func UDPComponentReceive(ctx *network.ComponentContext) error {
	switch ctx.Message().(type) {
	case *protobuf.ProxyResponse:
		log.Info("Node(udp) public ip is:", ctx.Message().(*protobuf.ProxyResponse).ProxyAddress)
		ctx.Network().ID.Address = "udp://" + ctx.Message().(*protobuf.ProxyResponse).ProxyAddress
		ctx.Network().FinishProxyServer("udp")
	}

	return nil
}
