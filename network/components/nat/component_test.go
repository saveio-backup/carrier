package nat

import (
	"testing"
	"time"

	"github.com/saveio/carrier/network"
	"github.com/saveio/carrier/network/discovery"

	"github.com/stretchr/testify/assert"
	"github.com/saveio/themis/common/log"
	"github.com/golang/glog"
)

func TestNatConnect(t *testing.T) {
	t.Parallel()

	numNodes := 2
	nodes := make([]*network.Network, 0)
	for i := 0; i < numNodes; i++ {
		b := network.NewBuilder()
		port := network.GetRandomUnusedPort()
		glog.Infof("randomUnusedPort:%d ",port)
		b.SetAddress(network.FormatAddress("tcp", "localhost", uint16(port)))
		RegisterComponent(b)
		b.AddComponent(new(discovery.Component))
		n, err := b.Build()
		go n.Listen()

		assert.Equal(t, nil, err)
		nodes = append(nodes, n)
		n.BlockUntilListening()
	}

	nodes[1].Bootstrap(nodes[0].Address)
	ComponentInt, ok := nodes[1].Component(discovery.ComponentID)
	assert.Equal(t, true, ok)
	Component := ComponentInt.(*discovery.Component)
	routes := Component.Routes
	peers := routes.GetPeers()
	for len(peers) < numNodes-1 {
		peers = routes.GetPeers()
		log.Info("peers: ",peers)
		time.Sleep(50 * time.Millisecond)
	}

	assert.Equal(t, len(peers), 1)
}
