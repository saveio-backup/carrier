package p2p

import (
	"github.com/saveio/carrier/network"
	"github.com/saveio/carrier/examples/channel_test/common/messages"
)

type NetComponent struct {
	*network.Component
	Net *Network
}

func (this *NetComponent) Startup(net *network.Network) {
}

func (this *NetComponent) Receive(ctx *network.ComponentContext) error {
	msg := ctx.Message()
	addr := ctx.Client().Address

	switch msg.(type) {
	case *messages.Message:
		this.Net.Receive(msg, addr)
	case *messages.Delivered:
		this.Net.Receive(msg, addr)
	}
	return nil
}
