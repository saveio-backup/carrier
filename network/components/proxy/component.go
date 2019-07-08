package proxy

import (
	"github.com/saveio/carrier/network"
)

type UDPProxyComponent struct {
	*network.Component
}

// Startup implements the Component callback
func (p *UDPProxyComponent) Startup(n *network.Network) {
	UDPComponentStartup(n)
}

func (p *UDPProxyComponent) Receive(ctx *network.ComponentContext) error {
	UDPComponentReceive(ctx)
	return nil
}

type KCPProxyComponent struct {
	*network.Component
}

// Startup implements the Component callback
func (p *KCPProxyComponent) Startup(n *network.Network) {
	KCPComponentStartup(n)
}

func (p *KCPProxyComponent) Receive(ctx *network.ComponentContext) error {
	KCPComponentReceive(ctx)
	return nil
}

type QuicProxyComponent struct {
	*network.Component
}

// Startup implements the Component callback
func (p *QuicProxyComponent) Startup(n *network.Network) {
	QuicComponentStartup(n)
}

func (p *QuicProxyComponent) Receive(ctx *network.ComponentContext) error {
	QuicComponentReceive(ctx)
	return nil
}

type TcpProxyComponent struct {
	*network.Component
}

// Startup implements the Component callback
func (p *TcpProxyComponent) Startup(n *network.Network) {
	TcpComponentStartup(n)
}

func (p *TcpProxyComponent) Receive(ctx *network.ComponentContext) error {
	TcpComponentReceive(ctx)
	return nil
}
