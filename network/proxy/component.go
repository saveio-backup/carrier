package proxy

import (
	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/carrier/network"
	"github.com/saveio/themis/common/log"
	"context"
	"net"
)

type ProxyComponent struct {
	*network.Component
}

func sendProxyRequest(n *network.Network) error {
	signed, err := n.PrepareMessage(context.Background(), &protobuf.ProxyRequest{})
	if err != nil {
		log.Error("failed to sign message in send proxy request")
		return err
	}

	buffer := n.BuildRawContent(signed)
	addrInfo, err := network.ParseAddress(n.GetProxyServer())
	if err != nil {
		log.Error("parse address error in send proxy request:",err.Error())
		return  err
	}
	resolved, err := net.ResolveUDPAddr("udp", addrInfo.HostPort())
	if err != nil {
		return err
	}
	_, err = n.Conn.WriteToUDP(buffer, resolved)

	if err != nil {
		log.Error("write error in proxy-component:",err.Error())
		return err
	}
	return nil
}

// Startup implements the Component callback
func (p *ProxyComponent) Startup(n *network.Network) {
	if err:=sendProxyRequest(n);err!=nil{
		log.Error("proxy compnent start-up error:", err.Error())
	}
}

func (p *ProxyComponent) Receive(ctx *network.ComponentContext) error {
	switch ctx.Message().(type) {
	case *protobuf.ProxyResponse:
		log.Info("Node public ip is:", ctx.Message().(*protobuf.ProxyResponse).ProxyAddress)

		relayIP:= "udp://" + ctx.Message().(*protobuf.ProxyResponse).ProxyAddress

		if relayIP == ctx.Network().ID.Address{
			ctx.Network().DeletePeerClient(ctx.Network().GetProxyServer())
		} else {
			ctx.Network().ID.Address = relayIP
		}
		ctx.Network().FinishProxyServer()
	}

	return nil
}
