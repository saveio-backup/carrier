/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-05-15 
*/
package proxy

import (
	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/carrier/network"
	"github.com/saveio/themis/common/log"
	"context"
	"net"
	"github.com/golang/protobuf/proto"
)

func prepareKCPRawMessage(n *network.Network, message proto.Message)([]byte, *net.UDPAddr, error) {
	signed, err := n.PrepareMessage(context.Background(), message)
	if err != nil {
		log.Error("failed to sign message in send proxy request")
		return nil, nil, err
	}

	buffer := n.BuildRawContent(signed)
	addrInfo, err := network.ParseAddress(n.GetProxyServer())
	if err != nil {
		log.Error("parse address error in send proxy request:",err.Error())
		return  nil, nil, err
	}
	resolved, err := net.ResolveUDPAddr("udp", addrInfo.HostPort())
	if err != nil {
		return nil, nil, err
	}
	return buffer, resolved, nil
}

func sendKCPProxyRequest(n *network.Network) error {
	buffer, resolved, err:= prepareKCPRawMessage(n, &protobuf.ProxyRequest{})
	if err!=nil{
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
func  KCPComponentStartup(n *network.Network) {
	client,_ := n.Client(n.GetProxyServer())
	client.Tell(context.Background(), &protobuf.ProxyRequest{})
}

func  KCPComponentReceive(ctx *network.ComponentContext) error {
	switch ctx.Message().(type) {
	case *protobuf.ProxyResponse:
		log.Info("Node public ip is:", ctx.Message().(*protobuf.ProxyResponse).ProxyAddress)

		relayIP:= "udp://" + ctx.Message().(*protobuf.ProxyResponse).ProxyAddress

		if relayIP == ctx.Network().ID.Address{
			ctx.Network().DeletePeerClient(ctx.Network().GetProxyServer())
		} else {
			ctx.Network().ID.Address = relayIP
		}
		ctx.Network().FinishProxyServer("kcp")
	}

	return nil
}
