package discovery

import (
	"context"
	"encoding/hex"
	"strings"
	"sync/atomic"

	"github.com/saveio/carrier/dht"
	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/carrier/network"
	"github.com/saveio/carrier/peer"
	"github.com/saveio/themis/common/log"
)

type Component struct {
	*network.Component

	DisablePing     bool
	DisablePong     bool
	DisableLookup   bool
	DisableAutoFind bool

	Routes *dht.RoutingTable
}

var (
	ComponentID                            = (*Component)(nil)
	_           network.ComponentInterface = (*Component)(nil)
)

// ComponentOption are configurable options for the discovery Component
type ComponentOption func(*Component)

func DisablePong() ComponentOption {
	return func(o *Component) {
		o.DisablePong = true
	}
}

func New(opts ...ComponentOption) *Component {
	p := new(Component)
	for _, opt := range opts {
		opt(p)
	}

	return p
}

func (state *Component) Startup(net *network.Network) {
	// Create routing table.
	state.Routes = dht.CreateRoutingTable(net.ID)
}

func (state *Component) Receive(ctx *network.ComponentContext) error {
	// Update routing for every incoming message.
	state.Routes.Update(ctx.Sender())
	gCtx := network.WithSignMessage(context.Background(), true)

	// Handle RPC.
	switch msg := ctx.Message().(type) {
	case *protobuf.Ping:
		if state.DisablePing {
			break
		}

		// Send pong to peer.
		err := ctx.Client().Tell(context.Background(), &protobuf.Pong{})
		if err != nil {
			return err
		}

		/*
			err := ctx.Reply(gCtx, &protobuf.Pong{})

			if err != nil {
				return err
			}
		*/

	case *protobuf.Pong:
		//[PoC] ensure bootstrap success with discovery component alone
		ctx.Client().PubKey = hex.EncodeToString(ctx.Sender().NetKey)
		if atomic.SwapUint32(&ctx.Client().RecvChannelClosed, 1) == 0 {
			ctx.Client().RecvRemotePubKey.Close()
		}

		if state.DisablePong {
			break
		}

		//todo delete for walking around
		peers := FindNode(ctx.Network(), ctx.Sender(), dht.BucketSize, 8)

		// Update routing table w/ closest peers to self.

		for _, peerID := range peers {
			state.Routes.Update(peerID)
		}

		log.Infof("bootstrapped w/ peer(s): %s.", strings.Join(state.Routes.GetPeerAddresses(), ", "))
	case *protobuf.LookupNodeRequest:
		if state.DisableLookup {
			break
		}

		// Prepare response.
		response := &protobuf.LookupNodeResponse{}

		// Respond back with closest peers to a provided target.
		for _, peerID := range state.Routes.FindClosestPeers(peer.ID(*msg.Target), dht.BucketSize) {
			id := protobuf.ID(peerID)
			response.Peers = append(response.Peers, &id)
		}

		err := ctx.Reply(gCtx, response)
		if err != nil {
			return err
		}

		log.Infof("connected peers: %s.", strings.Join(state.Routes.GetPeerAddresses(), ", "))
	case *protobuf.Disconnect:
		log.Info("receive disconnect signal from peer")
		ctx.Disconnect()
	}

	return nil
}

func (state *Component) Cleanup(net *network.Network) {
	// TODO: Save routing table?
}

func (state *Component) PeerDisconnect(client *network.PeerClient) {
	// Delete peer if in routing table.
	if client.ID != nil {
		if state.Routes.PeerExists(*client.ID) {
			state.Routes.RemovePeer(*client.ID)
			log.Infof("Peer %s has disconnected from %s.", client.ID.Address, client.Network.ID.Address)
		}
	}
}
