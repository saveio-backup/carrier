package network

import (
	"context"

	"github.com/saveio/carrier/crypto"
	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/carrier/peer"

	"net"

	"github.com/gogo/protobuf/proto"
)

// NetworkInterface represents a node in the network.
type NetworkInterface interface {

	// Init starts all network I/O workers.
	Init()

	// GetKeys() returns the keypair for this network
	GetKeys() *crypto.KeyPair

	// Listen starts listening for peers on a port.
	Listen()

	// Client either creates or returns a cached peer client given its host address.
	Client(address, peerID string) (*PeerClient, error)

	// BlockUntilListening blocks until this node is listening for new peers.
	BlockUntilListening()

	// Bootstrap with a number of peers and commence a handshake.
	Bootstrap(addresses []string) []string

	// Dial establishes a bidirectional connection to an address, and additionally handshakes with said address.
	Dial(address string, client *PeerClient) (interface{}, error)

	// Accept handles peer registration and processes incoming message streams.
	Accept(conn net.Conn, client *PeerClient)

	// Component returns a Components proxy interface should it be registered with the
	// network. The second returning parameter is false otherwise.
	//
	// Example: network.Component((*Component)(nil))
	Component(key interface{}) (ComponentInterface, bool)

	// PrepareMessage marshals a message into a *protobuf.Message and signs it with this
	// nodes private key. Errors if the message is null.
	PrepareMessage(ctx context.Context, message proto.Message) (*protobuf.Message, error)

	// Write asynchronously sends a message to a denoted target address.
	Write(address string, message *protobuf.Message) error

	// Broadcast asynchronously broadcasts a message to all peer clients.
	Broadcast(ctx context.Context, message proto.Message)

	// BroadcastByAddresses broadcasts a message to a set of peer clients denoted by their addresses.
	BroadcastByAddresses(ctx context.Context, message proto.Message, addresses ...string)

	// BroadcastByIDs broadcasts a message to a set of peer clients denoted by their peer IDs.
	BroadcastByIDs(ctx context.Context, message proto.Message, ids ...peer.ID)

	// BroadcastRandomly asynchronously broadcasts a message to random selected K peers.
	// Does not guarantee broadcasting to exactly K peers.
	BroadcastRandomly(ctx context.Context, message proto.Message, K int) []string

	// Close shuts down the entire network.
	Close()
}
