package addressmap

import (
	"github.com/golang/glog"
	"github.com/oniio/oniP2p/network"
	"github.com/oniio/oniP2p/peer"
)

type Component struct {
	*network.Component

	MappingAddress string
}

var (
	// ComponentID to reference address mapping Component
	ComponentID                            = (*Component)(nil)
	_           network.ComponentInterface = (*Component)(nil)
)

func (p *Component) Startup(n *network.Network) {
	glog.Infof("Setting up address mapping for address: %s", n.Address)

	info, err := network.ParseAddress(n.Address)
	if err != nil {
		glog.Errorf("error parsing network address %s\n", n.Address)
		return
	}

	mapInfo, err := network.ParseAddress(p.MappingAddress)
	if err != nil {
		glog.Errorf("error parsing mapping address %s\n", p.MappingAddress)
		return
	}

	glog.Infof("update mapping address from %s to %s", info.String(), mapInfo.String())

	n.Address = mapInfo.String()
	n.ID = peer.CreateID(n.Address, n.GetKeys().PublicKey)
}
