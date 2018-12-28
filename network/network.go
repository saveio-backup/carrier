package network

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oniio/oniP2p/crypto"
	"github.com/oniio/oniP2p/internal/protobuf"
	"github.com/oniio/oniP2p/network/transport"
	"github.com/oniio/oniP2p/peer"
	"github.com/oniio/oniP2p/types/opcode"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

type writeMode int

const (
	WRITE_MODE_LOOP writeMode = iota
	WRITE_MODE_DIRECT
)

const (
	defaultConnectionTimeout = 60 * time.Second
	defaultReceiveWindowSize = 4096
	defaultSendWindowSize    = 4096
	defaultWriteBufferSize   = 4096
	defaultRecvBufferSize    = 4 * 1024 * 1024
	defaultWriteFlushLatency = 50 * time.Millisecond
	defaultWriteTimeout      = 3 * time.Second
	defaultWriteMode         = WRITE_MODE_LOOP
)

var contextPool = sync.Pool{
	New: func() interface{} {
		return new(ComponentContext)
	},
}

var (
	_ NetworkInterface = (*Network)(nil)
)

// Network represents the current networking state for this node.
type Network struct {
	opts options

	// Node's keypair.
	keys *crypto.KeyPair

	// Full address to listen on. `protocol://host:port`
	Address string

	// Map of Components registered to the network.
	// map[string]Component
	Components *ComponentList

	// Node's cryptographic ID.
	ID peer.ID

	// Map of connection addresses (string) <-> *network.PeerClient
	// so that the Network doesn't dial multiple times to the same ip
	peers *sync.Map

	//RecvQueue chan *protobuf.Message

	// Map of connection addresses (string) <-> *ConnState
	connections *sync.Map

	// Map of protocol addresses (string) <-> *transport.Layer
	transports *sync.Map

	// listeningCh will block a goroutine until this node is listening for peers.
	listeningCh chan struct{}

	// <-kill will begin the server shutdown process
	kill chan struct{}
}

// options for network struct
type options struct {
	connectionTimeout time.Duration
	signaturePolicy   crypto.SignaturePolicy
	hashPolicy        crypto.HashPolicy
	recvWindowSize    int
	sendWindowSize    int
	writeBufferSize   int
	recvBufferSize    int
	writeFlushLatency time.Duration
	writeTimeout      time.Duration
	writeMode         writeMode
}

// ConnState represents a connection.
type ConnState struct {
	conn         net.Conn
	writer       *bufio.Writer
	messageNonce uint64
	writerMutex  *sync.Mutex
}

// Init starts all network I/O workers.
func (n *Network) Init() {
	// Spawn write flusher.
	go n.flushLoop()
}

func (n *Network) flushLoop() {
	t := time.NewTicker(n.opts.writeFlushLatency)
	defer t.Stop()
	for {
		select {
		case <-n.kill:
			return
		case <-t.C:
			n.connections.Range(func(key, value interface{}) bool {
				if state, ok := value.(*ConnState); ok {
					state.writerMutex.Lock()
					if err := state.writer.Flush(); err != nil {
						glog.Warning(err)
					}
					state.writerMutex.Unlock()
				}
				return true
			})
		}
	}
}

// GetKeys returns the keypair for this network
func (n *Network) GetKeys() *crypto.KeyPair {
	return n.keys
}

func (n *Network) dispatchMessage(client *PeerClient, msg *protobuf.Message) {
	if !client.IsIncomingReady() {
		return
	}

	var ptr proto.Message
	// unmarshal message based on specified opcode
	code := opcode.Opcode(msg.Opcode)
	switch code {
	case opcode.BytesCode:
		ptr = &protobuf.Bytes{}
	case opcode.PingCode:
		ptr = &protobuf.Ping{}
	case opcode.PongCode:
		ptr = &protobuf.Pong{}
	case opcode.LookupNodeRequestCode:
		ptr = &protobuf.LookupNodeRequest{}
	case opcode.LookupNodeResponseCode:
		ptr = &protobuf.LookupNodeResponse{}
	case opcode.UnregisteredCode:
		glog.Error("network: message received had no opcode")
		return
	default:
		var err error
		ptr, err = opcode.GetMessageType(code)
		if err != nil {
			glog.Error("network: received message opcode is not registered")
			return
		}
	}
	if len(msg.Message) > 0 {
		if err := proto.Unmarshal(msg.Message, ptr); err != nil {
			glog.Error(err)
			return
		}
	}

	client.Time = time.Now()

	if msg.RequestNonce > 0 && msg.ReplyFlag {
		if _state, exists := client.Requests.Load(msg.RequestNonce); exists {
			state := _state.(*RequestState)
			select {
			case state.data <- ptr:
			case <-state.closeSignal:
			}
			return
		}
	}

	switch msgRaw := ptr.(type) {
	case *protobuf.Bytes:
		client.handleBytes(msgRaw.Data)
	default:
		ctx := contextPool.Get().(*ComponentContext)
		ctx.client = client
		ctx.message = msgRaw
		ctx.nonce = msg.RequestNonce

		go func() {
			// Execute 'on receive message' callback for all Components.
			n.Components.Each(func(Component ComponentInterface) {
				if err := Component.Receive(ctx); err != nil {
					glog.Errorf("%+v", err)
				}
			})

			contextPool.Put(ctx)
		}()
	}
}

// Listen starts listening for peers on a port.
func (n *Network) Listen() {

	// Handle 'network starts listening' callback for Components.
	n.Components.Each(func(Component ComponentInterface) {
		Component.Startup(n)
	})

	// Handle 'network stops listening' callback for Components.
	defer func() {
		n.Components.Each(func(Component ComponentInterface) {
			Component.Cleanup(n)
		})
	}()

	addrInfo, err := ParseAddress(n.Address)
	if err != nil {
		glog.Fatal(err)
	}

	var listener net.Listener

	if t, exists := n.transports.Load(addrInfo.Protocol); exists {
		listener, err = t.(transport.Layer).Listen(int(addrInfo.Port))
		if err != nil {
			glog.Fatal(err)
		}
	} else {
		glog.Fatal("invalid protocol: " + addrInfo.Protocol)
	}

	n.startListening()

	glog.Infof("Listening for peers on %s.\n", n.Address)

	// handle server shutdowns
	go func() {
		select {
		case <-n.kill:
			// cause listener.Accept() to stop blocking so it can continue the loop
			listener.Close()
		}
	}()

	// Handle new clients.
	for {
		if conn, err := listener.Accept(); err == nil {
			go n.Accept(conn)

		} else {
			// if the Shutdown flag is set, no need to continue with the for loop
			select {
			case <-n.kill:
				glog.Infof("Shutting down server on %s.\n", n.Address)
				return
			default:
				glog.Error(err)
			}
		}
	}
}

// getOrSetPeerClient either returns a cached peer client or creates a new one given a net.Conn
// or dials the client if no net.Conn is provided.
func (n *Network) getOrSetPeerClient(address string, conn net.Conn) (*PeerClient, error) {
	address, err := ToUnifiedAddress(address)
	if err != nil {
		return nil, err
	}

	if address == n.Address {
		return nil, errors.New("network: peer should not dial itself")
	}

	// if conn is not nil, check that the sender host matches the net.Conn remote host address
	if conn != nil {
		addrInfo, err := ParseAddress(address)
		if err != nil {
			return nil, err
		}

		remoteAddrInfo, err := ParseAddress(fmt.Sprintf("%s://%s", conn.RemoteAddr().Network(), conn.RemoteAddr().String()))
		if err != nil {
			return nil, err
		}

		// disalbe checking for address matching to pass for the case when ip mapping address is used
		// to be further checked for impact
		if addrInfo.Host != remoteAddrInfo.Host {
			//return nil, errors.New("network: sender address did not match connection remote address")
		}
	}

	clientNew, err := createPeerClient(n, address)
	if err != nil {
		return nil, err
	}

	c, exists := n.peers.LoadOrStore(address, clientNew)
	if exists {
		client := c.(*PeerClient)

		if !client.IsOutgoingReady() {
			return nil, errors.New("network: peer failed to connect")
		}

		return client, nil
	}

	client := c.(*PeerClient)
	defer func() {
		client.setOutgoingReady()
	}()

	if conn == nil {
		conn, err = n.Dial(address)
		if err != nil {
			n.peers.Delete(address)
			return nil, err
		}
	}

	n.connections.Store(address, &ConnState{
		conn:        conn,
		writer:      bufio.NewWriterSize(conn, n.opts.writeBufferSize),
		writerMutex: new(sync.Mutex),
	})

	client.Init()

	return client, nil
}

// Client either creates or returns a cached peer client given its host address.
func (n *Network) Client(address string) (*PeerClient, error) {
	return n.getOrSetPeerClient(address, nil)
}

// ConnectionStateExists returns true if network has a connection on a given address.
func (n *Network) ConnectionStateExists(address string) bool {
	_, ok := n.connections.Load(address)
	return ok
}

// ConnectionState returns a connections state for current address.
func (n *Network) ConnectionState(address string) (*ConnState, bool) {
	conn, ok := n.connections.Load(address)
	if !ok {
		return nil, false
	}
	return conn.(*ConnState), true
}

// startListening will start node for listening for new peers.
func (n *Network) startListening() {
	close(n.listeningCh)
}

// BlockUntilListening blocks until this node is listening for new peers.
func (n *Network) BlockUntilListening() {
	<-n.listeningCh
}

// Bootstrap with a number of peers and commence a handshake.
func (n *Network) Bootstrap(addresses ...string) {
	n.BlockUntilListening()

	addresses = FilterPeers(n.Address, addresses)

	for _, address := range addresses {
		client, err := n.Client(address)

		if err != nil {
			glog.Error(err)
			continue
		}

		err = client.Tell(context.Background(), &protobuf.Ping{})
		if err != nil {
			continue
		}
	}
}

// Dial establishes a bidirectional connection to an address, and additionally handshakes with said address.
func (n *Network) Dial(address string) (net.Conn, error) {
	addrInfo, err := ParseAddress(address)
	if err != nil {
		return nil, err
	}

	if addrInfo.Host != "127.0.0.1" {
		host, err := ParseAddress(n.Address)
		if err != nil {
			return nil, err
		}
		// check if dialing address is same as its own IP
		if addrInfo.Host == host.Host {
			addrInfo.Host = "127.0.0.1"
		}
	}

	// Choose scheme.
	t, exists := n.transports.Load(addrInfo.Protocol)
	if !exists {
		glog.Fatal("invalid protocol: " + addrInfo.Protocol)
	}

	var conn net.Conn
	conn, err = t.(transport.Layer).Dial(addrInfo.HostPort())
	if err != nil {
		return nil, err
	}

	// use the connection for also receiving messages
	go n.Accept(conn)

	return conn, nil
}

// Accept handles peer registration and processes incoming message streams.
func (n *Network) Accept(incoming net.Conn) {
	var client *PeerClient
	var clientInit sync.Once

	recvWindow := NewRecvWindow(n.opts.recvWindowSize)

	// Cleanup connections when we are done with them.
	defer func() {
		time.Sleep(1 * time.Second)

		if client != nil {
			client.Close()
		}

		if incoming != nil {
			incoming.Close()
		}
	}()

	for {
		msg, err := n.receiveMessage(incoming)
		if err != nil {
			if err != errEmptyMsg {
				glog.Error(err)
			}
			break
		}

		// Initialize client if not exists.
		clientInit.Do(func() {
			client, err = n.getOrSetPeerClient(msg.Sender.Address, incoming)
			if err != nil {
				return
			}

			client.ID = (*peer.ID)(msg.Sender)

			if !n.ConnectionStateExists(client.ID.Address) {
				err = errors.New("network: failed to load session")
			}

			client.setIncomingReady()
		})

		if err != nil {
			glog.Error(err)
			return
		}

		go func() {
			// Peer sent message with a completely different ID. Disconnect.
			if !client.ID.Equals(peer.ID(*msg.Sender)) {
				glog.Errorf("message signed by peer %s but client is %s", peer.ID(*msg.Sender), client.ID.Address)
				return
			}

			recvWindow.Push(msg.MessageNonce, msg)

			ready := recvWindow.Pop()
			for _, msg := range ready {
				msg := msg
				client.Submit(func() {
					n.dispatchMessage(client, msg.(*protobuf.Message))
				})
			}
		}()
	}
}

// Component returns a Components proxy interface should it be registered with the
// network. The second returning parameter is false otherwise.
//
// Example: network.Component((*Component)(nil))
func (n *Network) Component(key interface{}) (ComponentInterface, bool) {
	return n.Components.Get(key)
}

// PrepareMessage marshals a message into a *protobuf.Message and signs it with this
// nodes private key. Errors if the message is null.
func (n *Network) PrepareMessage(ctx context.Context, message proto.Message) (*protobuf.Message, error) {
	if message == nil {
		return nil, errors.New("network: message is null")
	}

	opcode, err := opcode.GetOpcode(message)
	if err != nil {
		return nil, err
	}

	raw, err := proto.Marshal(message)
	if err != nil {
		return nil, err
	}

	id := protobuf.ID(n.ID)

	msg := &protobuf.Message{
		Message: raw,
		Opcode:  uint32(opcode),
		Sender:  &id,
	}

	if GetSignMessage(ctx) {
		signature, err := n.keys.Sign(
			n.opts.signaturePolicy,
			n.opts.hashPolicy,
			SerializeMessage(&id, raw),
		)
		if err != nil {
			return nil, err
		}
		msg.Signature = signature
	}

	return msg, nil
}

// Write asynchronously sends a message to a denoted target address.
func (n *Network) Write(address string, message *protobuf.Message) error {
	state, ok := n.ConnectionState(address)
	if !ok {
		return errors.New("network: connection does not exist")
	}

	message.MessageNonce = atomic.AddUint64(&state.messageNonce, 1)

	state.conn.SetWriteDeadline(time.Now().Add(n.opts.writeTimeout))

	err := n.sendMessage(state.writer, message, state.writerMutex)
	if err != nil {
		return err
	}

	if n.opts.writeMode == WRITE_MODE_DIRECT {
		state.writerMutex.Lock()
		if err := state.writer.Flush(); err != nil {
			glog.Warning(err)
		}
		state.writerMutex.Unlock()
	}
	return nil
}

// Broadcast asynchronously broadcasts a message to all peer clients.
func (n *Network) Broadcast(ctx context.Context, message proto.Message) {
	signed, err := n.PrepareMessage(ctx, message)
	if err != nil {
		glog.Errorf("network: failed to broadcast message")
		return
	}

	n.EachPeer(func(client *PeerClient) bool {
		err := n.Write(client.Address, signed)
		if err != nil {
			glog.Warningf("failed to send message to peer %v [err=%s]", client.ID, err)
		}
		return true
	})
}

// BroadcastByAddresses broadcasts a message to a set of peer clients denoted by their addresses.
func (n *Network) BroadcastByAddresses(ctx context.Context, message proto.Message, addresses ...string) {
	signed, err := n.PrepareMessage(ctx, message)
	if err != nil {
		return
	}

	for _, address := range addresses {
		n.Write(address, signed)
	}
}

// BroadcastByIDs broadcasts a message to a set of peer clients denoted by their peer IDs.
func (n *Network) BroadcastByIDs(ctx context.Context, message proto.Message, ids ...peer.ID) {
	signed, err := n.PrepareMessage(ctx, message)
	if err != nil {
		return
	}

	for _, id := range ids {
		n.Write(id.Address, signed)
	}
}

// BroadcastRandomly asynchronously broadcasts a message to random selected K peers.
// Does not guarantee broadcasting to exactly K peers.
func (n *Network) BroadcastRandomly(ctx context.Context, message proto.Message, K int) {
	var addresses []string

	n.EachPeer(func(client *PeerClient) bool {
		addresses = append(addresses, client.Address)

		// Limit total amount of addresses in case we have a lot of peers.
		return len(addresses) <= K*3
	})

	// Flip a coin and shuffle :).
	rand.Shuffle(len(addresses), func(i, j int) {
		addresses[i], addresses[j] = addresses[j], addresses[i]
	})

	if len(addresses) < K {
		K = len(addresses)
	}

	n.BroadcastByAddresses(ctx, message, addresses[:K]...)
}

// Close shuts down the entire network.
func (n *Network) Close() {
	close(n.kill)

	n.EachPeer(func(client *PeerClient) bool {
		client.Close()
		return true
	})
}

func (n *Network) EachPeer(fn func(client *PeerClient) bool) {
	n.peers.Range(func(_, value interface{}) bool {
		client := value.(*PeerClient)
		return fn(client)
	})
}
