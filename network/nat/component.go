package nat

import (
	"net"
	"time"

	"github.com/fd/go-nat"
	"github.com/golang/glog"
	"github.com/oniio/oniP2p/network"
	"github.com/oniio/oniP2p/peer"
)

type Component struct {
	*network.Component

	gateway nat.NAT

	internalIP net.IP
	externalIP net.IP

	internalPort int
	externalPort int
}

var (
	// ComponentID to reference NAT Component
	ComponentID                            = (*Component)(nil)
	_           network.ComponentInterface = (*Component)(nil)
)

func (p *Component) Startup(n *network.Network) {
	glog.Infof("Setting up NAT traversal for address: %s", n.Address)

	info, err := network.ParseAddress(n.Address)
	if err != nil {
		return
	}

	p.internalPort = int(info.Port)

	gateway, err := nat.DiscoverGateway()
	if err != nil {
		glog.Warning("Unable to discover gateway: ", err)
		return
	}

	p.internalIP, err = gateway.GetInternalAddress()
	if err != nil {
		glog.Warning("Unable to fetch internal IP: ", err)
		return
	}

	p.externalIP, err = gateway.GetExternalAddress()
	if err != nil {
		glog.Warning("Unable to fetch external IP: ", err)
		return
	}

	glog.Infof("Discovered gateway following the protocol %s.", gateway.Type())

	glog.Info("Internal IP: ", p.internalIP.String())
	glog.Info("External IP: ", p.externalIP.String())

	p.externalPort, err = gateway.AddPortMapping("tcp", p.internalPort, "noise", 1*time.Second)

	if err != nil {
		glog.Warning("Cannot setup port mapping: ", err)
		return
	}

	glog.Infof("External port %d now forwards to your local port %d.", p.externalPort, p.internalPort)

	p.gateway = gateway

	info.Host = p.externalIP.String()
	info.Port = uint16(p.externalPort)

	// Set peer information based off of port mapping info.
	n.Address = info.String()
	n.ID = peer.CreateID(n.Address, n.GetKeys().PublicKey)

	glog.Infof("Other peers may connect to you through the address %s.", n.Address)
}

func (p *Component) Cleanup(n *network.Network) {
	if p.gateway != nil {
		glog.Info("Removing port binding...")

		err := p.gateway.DeletePortMapping("tcp", p.internalPort)
		if err != nil {
			glog.Error(err)
		}
	}
}

// RegisterComponent registers a Component that automates port-forwarding of this nodes
// listening socket through any available UPnP interface.
//
// The Component is registered with a priority of -999999, and thus is executed first.
func RegisterComponent(builder *network.Builder) {
	builder.AddComponentWithPriority(-99999, new(Component))
}
