package network

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/saveio/carrier/crypto"
	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/carrier/network/transport"
	"github.com/saveio/carrier/peer"
	"github.com/saveio/carrier/types/opcode"

	"os"
	"os/signal"
	"syscall"

	"sync/atomic"

	"reflect"

	"encoding/hex"

	"github.com/gogo/protobuf/proto"
	"github.com/lucas-clemente/quic-go"
	"github.com/pkg/errors"
	"github.com/saveio/carrier/metric/carrier-metrics"
	"github.com/saveio/themis/common/log"
)

var Version string

type PeerState int

const (
	PEER_UNKNOWN     PeerState = 0 // peer network unknown
	PEER_UNREACHABLE PeerState = 1 // peer network unreachable
	PEER_REACHABLE   PeerState = 2 // peer network reachable
)

const (
	defaultConnectionTimeout = 60 * time.Second
	defaultReceiveWindowSize = 4096
	defaultSendWindowSize    = 4096
	defaultWriteBufferSize   = 1024 * 1024 * 8
	defaultRecvBufferSize    = 1024 * 1024 * 8
	defaultWriteFlushLatency = 10 * time.Millisecond
	defaultWriteTimeout      = 3
	defaultProxyNotifySize   = 256
	defaultCompressFileSize  = 4 * 1024 * 1024
	defaultStreamQueueLen    = 256
)

const (
	PER_SEND_BLOCK_SIZE = 1024 * 128
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
	netID uint32

	opts options

	// Node's keypair.
	keys *crypto.KeyPair

	//Store relationship between clientID(for example public key) and IP
	ClientID *sync.Map

	// Full address to listen on. `protocol://host:port`
	Address    string
	ListenAddr string
	// Map of Components registered to the network.
	// map[string]Component
	Components *ComponentList

	// Node's cryptographic ID.
	ID      peer.ID
	ConnMgr struct {
		sync.Mutex
		// Map of connection addresses (string) <-> *network.PeerClient
		// so that the Network doesn't dial multiple times to the same ip
		peers *sync.Map
		// Map of connection addresses (string) <-> *ConnState
		connections *sync.Map

		connStates *sync.Map
		streams    *sync.Map
	}

	Conn *net.UDPConn

	// Map of protocol addresses (string) <-> *transport.Layer
	transports *sync.Map

	// listeningCh will block a goroutine until this node is listening for peers.
	listeningCh chan struct{}

	// <-kill will begin the server shutdown process
	Kill chan struct{}

	ProxyService Proxy

	dialTimeout time.Duration

	compressEnable    bool
	compressAlgo      AlgoType
	CompressCondition CompressCondition

	createClientMutex           *sync.Mutex
	NetDistanceMetric           *sync.Map
	metric                      Metric
	DisableDispatchMsgGoroutine bool
	Reporter                    *metrics.BandwidthCounter

	streamQueueLen uint16
}

type NetDistanceMetric struct {
	StartTime      int64
	PackageCounter uint64
	TimeCounter    uint64
}

type ProxyEvent struct {
	Address      string
	ConnectionID string
}

type ProxyServer struct {
	IP     string
	PeerID string
}

type Proxy struct {
	Enable          bool
	Finish          *sync.Map
	Servers         []ProxyServer
	WorkID          uint16
	ConnectionEvent []chan *ProxyEvent
}

// options for network struct
type options struct {
	connectionTimeout    time.Duration
	signaturePolicy      crypto.SignaturePolicy
	hashPolicy           crypto.HashPolicy
	recvWindowSize       int
	sendWindowSize       int
	writeBufferSize      int
	recvBufferSize       int
	writeFlushLatency    time.Duration
	perBlockWriteTimeout int
}

// ConnState represents a connection.
type ConnState struct {
	conn           interface{}
	writer         *bufio.Writer
	messageNonce   uint64
	writerMutex    *sync.Mutex
	DataSignal     chan *protobuf.Message
	ControllSignal chan *protobuf.Message
}

// Init starts all network I/O workers.
func (n *Network) Init() {
	// Spawn write flusher.
	//go n.flushLoop()
	go n.waitExit()
}

func (n *Network) waitExit() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	select {
	case sig := <-sigs:
		log.Infof("Network received exit signal:%v.", sig.String())
		n.Close()
		os.Exit(0)
	}
}

func (n *Network) PeerID() string {
	return fmt.Sprintf("%s", n.ID.Id)
}

// GetKeys returns the keypair for this network
func (n *Network) GetKeys() *crypto.KeyPair {
	return n.keys
}

func (n *Network) dispatchMessage(client *PeerClient, msg *protobuf.Message) {
	if !client.IsIncomingReady() {
		log.Warnf("in Network.dispatchMessage, client.IsIncomingReady is false,client addr:%s,msg.ID:%s,msg.Nonce:%d", client.Address, msg.MessageID, msg.MessageNonce)
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
	case opcode.DisconnectCode:
		ptr = &protobuf.Disconnect{}
	case opcode.ProxyRequestCode:
		ptr = &protobuf.ProxyRequest{}
	case opcode.ProxyResponseCode:
		ptr = &protobuf.ProxyResponse{}
	case opcode.KeepaliveCode:
		ptr = &protobuf.Keepalive{}
	case opcode.KeepaliveResponseCode:
		ptr = &protobuf.KeepaliveResponse{}
	case opcode.UnregisteredCode:
		log.Error("network: message received had no opcode")
		return
	default:
		var err error
		ptr, err = opcode.GetMessageType(code)
		if err != nil {
			log.Error("network: received message opcode is not registered")
			return
		}
	}
	if len(msg.Message) > 0 {
		if err := proto.Unmarshal(msg.Message, ptr); err != nil {
			log.Error(err)
			return
		}
	} else {
		if msg.Opcode >= 1000 {
			log.Warn("in dispatch, msg.Message length is zero. maybe this swith is ERROR, pls CHECK. msg.Nonce:", msg.MessageNonce, ",msg.Sender:", msg.Sender.Address, ",msg.Opcode:", msg.Opcode)
		}
	}

	client.Time = time.Now()

	if msg.RequestNonce > 0 && msg.ReplyFlag {
		log.Infof("in Netowrk.dispatch, msg.RequestNonce:%d, msg.ReplyFlag:%d, msg.MessageNonce:%d,msg.Sender:%s", msg.RequestNonce, msg.ReplyFlag, msg.MessageNonce, msg.Sender.Address)
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

		if n.DisableDispatchMsgGoroutine {
			// Execute 'on receive message' callback for all Components.
			n.Components.Each(func(Component ComponentInterface) {
				if msg.Opcode >= 1000 {
					log.Infof("in Network Component.Receive handle, recv msg.opcode:%d, msg.Nonce:%d, component name:%s,msg.Sender:%s", msg.Opcode, msg.MessageNonce, reflect.TypeOf(Component), msg.Sender.Address)
				}
				if err := Component.Receive(ctx); err != nil {
					log.Errorf("in network Component.Receive err:%+v, component name:%s,msg.opcode:%d, msg.Nonce:%d, msg.Sender:%s", err, reflect.TypeOf(Component), msg.Opcode, msg.MessageNonce, msg.Sender.Address)
				}
			})

			contextPool.Put(ctx)
			return
		}

		go func() {
			// Execute 'on receive message' callback for all Components.
			n.Components.Each(func(Component ComponentInterface) {
				if msg.Opcode >= 1000 {
					log.Infof("in Network Component.Receive handle, recv msg.opcode:%d, msg.Nonce:%d, component name:%s,msg.Sender:%s", msg.Opcode, msg.MessageNonce, reflect.TypeOf(Component), msg.Sender.Address)
				}
				if err := Component.Receive(ctx); err != nil {
					log.Errorf("in network Component.Receive err:%+v, component name:%s,msg.opcode:%d, msg.Nonce:%d, msg.Sender:%s", err, reflect.TypeOf(Component), msg.Opcode, msg.MessageNonce, msg.Sender.Address)
				}
			})

			contextPool.Put(ctx)
		}()
	}
}

// Listen starts listening for peers on a port.
func (n *Network) Listen() {
	addrInfo, err := ParseAddress(n.ListenAddr)
	if err != nil {
		log.Fatal("parse addr:", addrInfo.String(), err)
		return
	}

	var listener interface{}
	if t, exists := n.transports.Load(addrInfo.Protocol); exists {
		listener, err = t.(transport.Layer).Listen(addrInfo.HostPort())
		if udpconn, ok := listener.(*net.UDPConn); ok {
			n.Conn = udpconn
		}
		if err != nil {
			log.Fatal("in (carrier) Network.Listen(), listen ip:", addrInfo.String(), err)
			return
		}
	} else {
		log.Fatal("invalid protocol: " + addrInfo.Protocol)
		return
	}
	// Handle 'network starts listening' callback for Components.
	n.Components.Each(func(Component ComponentInterface) {
		Component.Startup(n)
	})
	n.startListening()
	// Handle 'network stops listening' callback for Components.
	defer func() {
		n.Components.Each(func(Component ComponentInterface) {
			Component.Cleanup(n)
		})
	}()

	// handle server shutdowns
	go func() {
		select {
		case <-n.Kill:
			// cause listener.Accept() to stop blocking so it can continue the loop
			if addrInfo.Protocol == "tcp" || addrInfo.Protocol == "kcp" {
				listener.(net.Listener).Close()
			}
			if addrInfo.Protocol == "udp" {
				udpConn, _ := listener.(*net.UDPConn)
				udpConn.Close()
			}
			if addrInfo.Protocol == "quic" {
				listener.(quic.Listener).Close()
			}
		}
	}()

	// Handle new clients.
	switch addrInfo.Protocol {
	case "tcp", "kcp":
		n.netListen(listener)
	case "udp":
		go n.AcceptUdp(listener.(*net.UDPConn))
		select {}
	case "quic":
		n.quicListen(listener)

	default:
		log.Fatal("invalid protocol: " + addrInfo.Protocol)
	}
}

func (n *Network) netListen(listener interface{}) {
	for {
		if conn, err := listener.(net.Listener).Accept(); err == nil {
			go n.Accept(conn, nil)

		} else {
			// if the Shutdown flag is set, no need to continue with the for loop
			select {
			case <-n.Kill:
				log.Infof("Shutting down server on %s.", n.Address)
				return
			default:
				log.Error(err)
				return
			}
		}
	}
}

func (n *Network) quicListen(listener interface{}) {
	for {
		if session, err := listener.(quic.Listener).Accept(context.Background()); err == nil {
			stream, err := session.AcceptStream(context.Background())
			if err != nil {
				log.Error("Open stream sync in session is err:", err.Error())
				return
			}
			go n.AcceptQuic(stream, nil)

		} else {
			// if the Shutdown flag is set, no need to continue with the for loop
			select {
			case <-n.Kill:
				log.Infof("Shutting down server on %s.", n.Address)
				return
			default:
				log.Error("quic accept err:", err)
				return
			}
		}
	}
}

func (n *Network) GetPeerClient(peerID string) *PeerClient {
	if client, ok := n.ConnMgr.peers.Load(peerID); ok {
		return client.(*PeerClient)
	} else {
		return nil
	}
}

func (n *Network) GetAddrByPeerID(peerID string) string {
	if peer := n.GetPeerClient(peerID); peer != nil {
		return peer.Address
	} else {
		return "NO_EXIST_ADDR"
	}
}

func (n *Network) resetConnMgrItemByPeerID(address, peerID string) {
	if peer, ok := n.ConnMgr.peers.Load(address); ok {
		n.ConnMgr.peers.Store(peerID, peer)
		n.ConnMgr.peers.Delete(address)
	} else {
		return
	}

	if conn, ok := n.ConnMgr.connections.Load(address); ok {
		n.ConnMgr.connections.Store(peerID, conn)
		n.ConnMgr.connections.Delete(address)
	} else {
		log.Errorf("addr:%s,peerID:%s, peer exist but connection does not exist", address, peerID)
		return
	}

	if status, ok := n.ConnMgr.connStates.Load(address); ok {
		n.ConnMgr.connStates.Store(peerID, status)
		n.ConnMgr.connStates.Delete(address)
	} else {
		log.Errorf("addr:%s,peerID:%s, peer and connection exist but status does not exist", address, peerID)
		return
	}

	if stream, ok := n.ConnMgr.streams.Load(address); ok {
		n.ConnMgr.streams.Store(peerID, stream)
		n.ConnMgr.streams.Delete(address)
	} else {
		log.Errorf("addr:%s,peerID:%s, peer、connection、status exist but stream does not exist", address, peerID)
		return
	}
}

// getOrSetPeerClient either returns a cached peer client or creates a new one given a net.Conn
// or dials the client if no net.Conn is provided.
func (n *Network) getOrSetPeerClient(address, peerID string, conn interface{}) (*PeerClient, error) {
	n.createClientMutex.Lock()
	defer n.createClientMutex.Unlock()

	if peerID == "" {
		peerID = address //when start bootstrap, peerID is nil, we need to instead of it by address;
	}

	address, err := ToUnifiedAddress(address)
	if err != nil {
		return nil, err
	}

	if address == n.Address {
		return nil, errors.New("network: peer should not dial itself")
	}

	clientNew, err := createPeerClient(n, address)
	if err != nil {
		return nil, err
	}

	if address != peerID { //address!=peerID stand for
		n.resetConnMgrItemByPeerID(address, peerID)
	}

	c, exists := n.ConnMgr.peers.Load(peerID)
	if exists {
		client := c.(*PeerClient)

		if !client.IsOutgoingReady() {
			return nil, errors.New("network: peer failed to connect")
		}

		return client, nil
	}

	clientNew.PubKey = peerID
	client := clientNew
	defer func() {
		client.setOutgoingReady()
	}()

	n.ConnMgr.Mutex.Lock()
	defer n.ConnMgr.Mutex.Unlock()
	if conn == nil {
		conn, err = n.Dial(address, client)

		if err != nil {
			n.ConnMgr.peers.Delete(peerID)
			return nil, err
		}
	}
	n.initConnection(address, peerID, conn)
	n.ConnMgr.peers.Store(peerID, client)
	n.ConnMgr.streams.Store(peerID, NewMultiStream())
	client.Init()
	n.UpdateConnState(peerID, PEER_REACHABLE)

	client.setIncomingReady()
	return client, nil
}

func (n *Network) initConnection(address, peerID string, conn interface{}) {
	addrInfo, err := ParseAddress(address)
	if err != nil {
		log.Errorf("address:%s,initConnection err:%s", address, err.Error())
	}
	switch addrInfo.Protocol {
	case "tcp", "kcp":
		netConn, _ := conn.(net.Conn)
		n.ConnMgr.connections.Store(peerID, &ConnState{
			conn:           conn,
			writer:         bufio.NewWriterSize(netConn, n.opts.writeBufferSize),
			writerMutex:    new(sync.Mutex),
			DataSignal:     make(chan *protobuf.Message, 1),
			ControllSignal: make(chan *protobuf.Message, 1),
		})
	case "udp":
		udpConn, _ := conn.(*net.UDPConn)
		n.ConnMgr.connections.Store(peerID, &ConnState{
			conn:        conn,
			writer:      bufio.NewWriterSize(udpConn, n.opts.writeBufferSize),
			writerMutex: new(sync.Mutex),
		})
	case "quic":
		netConn, _ := conn.(quic.Stream)
		n.ConnMgr.connections.Store(peerID, &ConnState{
			conn:        conn,
			writer:      bufio.NewWriterSize(netConn, n.opts.writeBufferSize),
			writerMutex: new(sync.Mutex),
		})
	default:
		log.Error("does not support", addrInfo.Protocol, ", pls use kcp/udp/tcp/quic protocol.")
	}
}

// Client either creates or returns a cached peer client given its host address.
func (n *Network) Client(address, peerID string) (*PeerClient, error) {
	return n.getOrSetPeerClient(address, peerID, nil)
}

func (n *Network) ClientExist(peerID string) bool {
	_, ok := n.ConnMgr.peers.Load(peerID)
	return ok
}

// ConnectionStateExists returns true if network has a connection on a given address.
func (n *Network) ConnectionStateExists(peerID string) bool {
	_, ok := n.ConnMgr.connections.Load(peerID)
	return ok
}

func (n *Network) ProxyConnectionStateExists() (bool, error) {
	if n.ProxyModeEnable() == true {
		_, peerID := n.GetWorkingProxyServer()
		_, ok := n.ConnMgr.connections.Load(peerID)
		return ok, nil
	} else {
		return false, errors.New("proxy does not enable")
	}
}

// ConnectionState returns a connections state for current address.
func (n *Network) ConnectionState(peerID string) (*ConnState, bool) {
	conn, ok := n.ConnMgr.connections.Load(peerID)
	if !ok {
		return nil, false
	}
	return conn.(*ConnState), true
}

// ConnectionState returns a connections state for current address.
func (n *Network) ProxyConnectionState() (*ConnState, bool) {
	if n.ProxyModeEnable() == false {
		return nil, false
	}
	_, peerID := n.GetWorkingProxyServer()
	conn, ok := n.ConnMgr.connections.Load(peerID)
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

func (n *Network) BlockUntilProxyFinish(protocol string) {
	switch protocol {
	case "tcp":
		n.BlockUntilTcpProxyFinish()
	case "udp":
		n.BlockUntilUDPProxyFinish()
	case "quic":
		n.BlockUntilQuicProxyFinish()
	case "kcp":
		n.BlockUntilKCPProxyFinish()
	default:
		log.Error("proxy backOff blocked only support tcp/udp/kcp/quic.")
	}

}

func (n *Network) BlockUntilQuicProxyFinish() {
	if notify, ok := n.ProxyService.Finish.Load("quic"); ok {
		<-notify.(chan struct{})
	}
}

func (n *Network) BlockUntilTcpProxyFinish() {
	if notify, ok := n.ProxyService.Finish.Load("tcp"); ok {
		<-notify.(chan struct{})
	}
}

func (n *Network) BlockUntilKCPProxyFinish() {
	if notify, ok := n.ProxyService.Finish.Load("kcp"); ok {
		<-notify.(chan struct{})
	}
}

func (n *Network) BlockUntilUDPProxyFinish() {
	if notify, ok := n.ProxyService.Finish.Load("udp"); ok {
		<-notify.(chan struct{})
	}
}

func (n *Network) ReconnectProxyServer(address, peerID string) error {
	if client := n.GetPeerClient(peerID); client != nil {
		client.DisableBackoff()
		client.Close()
	}

	addrInfo, err := ParseAddress(address)
	if err != nil {
		log.Errorf("address:%s,reconnect proxy server parse address err:%s", address, err.Error())
	}
	n.ProxyService.Finish.Store(addrInfo.Protocol, make(chan struct{}))

	log.Infof("in carrier.Network,Reconnect Proxy Server start, proxy addr:%s", address)
	return n.ConnectProxyServer(address, peerID)
}

func (n *Network) ConnectProxyServer(address, peerID string) error {
	if n.ConnectionStateExists(address) {
		log.Info("in ConnectProxyServer, connection belong to addr:", address, "has exist, return directly.")
		return nil
	}

	client, err := n.Client(address, "")
	if err != nil {
		log.Error("create client in ConnectProxyServer err:", err, ";address:", address)
		return err
	}
	err = client.TellByAddr(context.Background(), &protobuf.ProxyRequest{})
	if err != nil {
		log.Error("new client send proxy request message in ConnectProxyServer err:", err, ";address:", address)
		return err
	}
	return nil
}

func (n *Network) ReconnectPeer(address string) (error, string) {
	if client := n.GetPeerClient(address); client != nil {
		client.DisableBackoff()
		client.Close()
	}

	log.Infof("in carrier.Network,Reconnect Peer start, peer addr:%s", address)
	return n.ConnectPeer(address)
}

func (n *Network) ConnectPeer(address string) (error, string) {
	if n.ConnectionStateExists(address) {
		log.Info("in ConnectPeer, connection belong to addr:", address, "has exist, return directly.")
		return nil, ""
	}

	if n.ClientExist(address) {
		log.Info("in ConnectPeer, client belong to addr:", address, "has exist, return directly.")
		return nil, ""
	}
	client, err := n.Client(address, "")
	if err != nil {
		log.Error("create client in ConnectPeer err:", err, ";address:", address)
		return err, ""
	}

	err = client.Tell(context.Background(), &protobuf.Ping{})
	if err != nil {
		log.Error("new client send ping message in ConnectPeer err:", err, ";address:", address)
		return err, ""
	} else {
		<-client.RecvRemotePubKey
	}
	return nil, client.ClientID()
}

// Bootstrap with a number of peers and commence a handshake.
func (n *Network) Bootstrap(addresses []string) []string {
	n.BlockUntilListening()

	addresses = FilterPeers(n.Address, addresses)
	clientID := make([]string, 0, len(addresses))
	for _, address := range addresses {
		client, err := n.Client(address, "")
		if err != nil {
			log.Error("create client in bootstrap err:", err, ";address:", address)
			continue
		}

		err = client.TellByAddr(context.Background(), &protobuf.Ping{})
		if err != nil {
			log.Error("new client send ping message err:", err, ";address:", address)
			continue
		} else {
			<-client.RecvRemotePubKey
			clientID = append(clientID, client.ClientID())
		}
	}
	return clientID
}

// Dial establishes a bidirectional connection to an address, and additionally handshakes with said address.
func (n *Network) Dial(address string, client *PeerClient) (interface{}, error) {
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
			//addrInfo.Host = "127.0.0.1"
		}
	}

	// Choose scheme.
	t, exists := n.transports.Load(addrInfo.Protocol)
	if !exists {
		log.Fatal("invalid protocol: " + addrInfo.Protocol)
	}

	var conn interface{}
	conn, err = t.(transport.Layer).Dial(addrInfo.HostPort(), n.dialTimeout)
	if err != nil {
		return nil, err
	}
	log.Info("in Network.Dial, dial success, dial to addr:", addrInfo.String(), ",local Network.ID.Address:", n.ID.Address)
	if addrInfo.Protocol == "tcp" || addrInfo.Protocol == "kcp" {
		go n.Accept(conn.(net.Conn), client)
	}
	if addrInfo.Protocol == "quic" {
		go n.AcceptQuic(conn.(quic.Stream), client)
	}
	return conn, nil
}

// AcceptQuic handles peer registration and processes incoming message streams.
// Notice: when Network.AcceptQuic is called by Listen goroutine, cli is nil;
// Notice: when Network.AcceptQuic is called by Dial handle-flow, we have to set client
// Notice: be relvant to the inbound connection
func (n *Network) AcceptQuic(stream quic.Stream, cli *PeerClient) {
	// Cleanup connections when we are done with them.
	defer func() {
		//time.Sleep(1 * time.Second)
		if cli != nil {
			log.Infof("in AcceptQuic, client:%s close.", cli.Address)
			cli.Close()
		}

		if stream != nil {
			log.Infof("in AcceptQuic, stream close.")
			stream.Close()
		}
	}()
	var address string
	log.Info("(quic)in Network.Accept, there is a new inbound connection in Quic.Listen,streamID:", stream.StreamID(), ", local addr:", n.ID.Address)
	for {
		var client *PeerClient
		msg, err := n.receiveQuicMessage(stream)
		if err != nil {
			log.Warnf("receive quic msg from %s err: %s", address, err.Error())
			log.Warn("quit connect with ", address)
			return
		}
		peerID := hex.EncodeToString(msg.Sender.NetKey)
		if n.ProxyModeEnable() {
			client, err = n.getOrSetPeerClient(msg.Sender.Address, peerID, nil)
		} else {
			client, err = n.getOrSetPeerClient(msg.Sender.Address, peerID, stream)
		}

		if err != nil {
			return
		}
		address = msg.Sender.Address
		client.ID = (*peer.ID)(msg.Sender)

		if !n.ConnectionStateExists(client.PeerID()) {
			err = errors.New("network: failed to load session")
		}

		//client.setIncomingReady() has done in getOrSetPeerClient() function
		//})

		if err != nil {
			log.Error(err)
			return
		}

		if msg.Signature != nil && !crypto.Verify(
			n.opts.signaturePolicy,
			n.opts.hashPolicy,
			msg.Sender.NetKey,
			SerializeMessage(msg.Sender, msg.Message),
			msg.Signature,
		) {
			log.Errorf("received message had an malformed signature")
			return
		}
		// Peer sent message with a completely different ID. Disconnect.
		if !client.ID.Equals(peer.ID(*msg.Sender)) {
			log.Errorf("message signed by peer %s but client is %s", peer.ID(*msg.Sender), client.ID.Address)
			return
		}

		/*		client.RecvWindow.Push(msg.MessageNonce, msg)
				ready := client.RecvWindow.Pop()
				for _, msg := range ready {
					msg := msg
					cli := client
					client.Submit(func() {
						n.dispatchMessage(cli, msg.(*protobuf.Message))
					})
				}*/
		n.dispatchMessage(client, msg)
	}
}

// Accept handles peer registration and processes incoming message streams.
// Notice: when Network.Accept is called by Listen goroutine, cli is nil;
// Notice: when Network.Accept is called by Dial handle-flow, we have to set client
// Notice: be relevant to the inbound connection
func (n *Network) Accept(incoming net.Conn, cli *PeerClient) {
	// Cleanup connections when we are done with them.
	defer func() {
		if cli != nil {
			log.Info("(tcp/kcp) Accept quit, client close now. client.addr:", cli.Address)
			cli.Close()
		}

		if incoming != nil {
			log.Infof("(tcp/kcp) Accept quit, inbound connection close now. conn.Local-Addr:%s, conn.Remote-Addr:%s", incoming.LocalAddr().String(), incoming.RemoteAddr().String())
			incoming.Close()
		}
	}()
	log.Info("(tcp/kcp)in Network.Accept, there is a new inbound connection in TCP.Listen,remote addr:", incoming.RemoteAddr().String(), ", local addr:", n.ID.Address)
	var client *PeerClient
	for {
		if client == nil {
			log.Debugf("receive msg from client.remoteAddr: %s", incoming.RemoteAddr().String())
		} else {
			log.Debugf("receive msg from client.addr: %s, client: %p, conn: %p", client.Address, client, incoming)
		}
		msg, err := n.receiveMessage(client, incoming)
		if err != nil {
			log.Error(err)
			break
		}

		peerID := hex.EncodeToString(msg.Sender.NetKey)
		if n.ProxyModeEnable() {
			client, err = n.getOrSetPeerClient(msg.Sender.Address, peerID, nil)
		} else {
			client, err = n.getOrSetPeerClient(msg.Sender.Address, peerID, incoming)
		}
		if err != nil {
			log.Error(err)
			return
		}

		client.ID = (*peer.ID)(msg.Sender)

		if !n.ConnectionStateExists(client.PeerID()) {
			log.Error("network: failed to load session")
			return
		}

		if msg.Signature != nil && !crypto.Verify(
			n.opts.signaturePolicy,
			n.opts.hashPolicy,
			msg.Sender.NetKey,
			SerializeMessage(msg.Sender, msg.Message),
			msg.Signature,
		) {
			log.Errorf("received message had an malformed signature")
			return
		}
		// Peer sent message with a completely different ID. Disconnect.
		if !client.ID.Equals(peer.ID(*msg.Sender)) {
			log.Errorf("message signed by peer %s but client is %s", peer.ID(*msg.Sender), client.ID.Address)
			return
		}
		if client.EnableAckReply && msg.MessageID != "" {
			err := client.Tell(context.Background(), &protobuf.AsyncAckResponse{SendTimestamp: int64(time.Now().Second()), MessageId: msg.MessageID})
			if err != nil {
				log.Errorf("reply ack to %s err:%s, messageID:%s", client.Address, err.Error(), msg.MessageID)
			} else {
				log.Debugf("reply ack to %s success, messageID:%s", client.Address, msg.MessageID)
			}
		}
		n.dispatchMessage(client, msg)
	}
}

// Accept handles peer registration and processes incoming message streams.
func (n *Network) AcceptUdp(incoming interface{}) {
	var client *PeerClient

	// Cleanup connections when we are done with them.
	defer func() {
		time.Sleep(1 * time.Second)

		if client != nil {
			client.Close()
		}

		if incoming != nil {
			udpConn, _ := incoming.(*net.UDPConn)
			udpConn.Close()
		}
	}()

	for {
		msg, err := n.receiveUDPMessage(incoming)
		if err != nil || msg == nil {
			if err != errEmptyMsg {
				log.Warn(err)
			}
			break
		}
		//log.Infof("(udp) receive from addr:%s,message.opcode:%d, message.sign:%s",msg.Sender.Address, msg.Opcode, hex.EncodeToString(msg.Signature))
		func() {
			if msg.Signature != nil && !crypto.Verify(
				n.opts.signaturePolicy,
				n.opts.hashPolicy,
				msg.Sender.NetKey,
				SerializeMessage(msg.Sender, msg.Message),
				msg.Signature,
			) {
				log.Error("received message had an malformed signature")
				return
			}
			peerID := hex.EncodeToString(msg.Sender.NetKey)
			if n.ProxyModeEnable() {
				client, err = n.getOrSetPeerClient(msg.Sender.Address, peerID, nil)
			} else {
				client, err = n.getOrSetPeerClient(msg.Sender.Address, peerID, incoming)
			}

			if err != nil {
				log.Error(err)
				return
			}

			client.ID = (*peer.ID)(msg.Sender)

			if !n.ConnectionStateExists(client.PeerID()) {
				log.Error(errors.New("network: failed to load session"))
				return
			}

			// Peer sent message with a completely different ID. Disconnect.
			if !client.ID.Equals(peer.ID(*msg.Sender)) {
				log.Errorf("message signed by peer %s but client is %s", peer.ID(*msg.Sender), client.ID.Address)
				return
			}
			cli := client
			client.Submit(func() {
				n.dispatchMessage(cli, msg)
			})
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
		NetID:   n.netID,
		NeedAck: false,
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

func (n *Network) PrepareMessageWithMsgID(ctx context.Context, message proto.Message, msgID string) (*protobuf.Message, error) {
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
		Message:   raw,
		Opcode:    uint32(opcode),
		Sender:    &id,
		NetID:     n.netID,
		MessageID: msgID,
		NeedAck:   true,
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

func (n *Network) writeToDispatchChannel(state *ConnState, message *protobuf.Message) {
	switch message.Opcode {
	case uint32(opcode.KeepaliveCode):
		state.ControllSignal <- message
	default:
		state.DataSignal <- message
	}
}

func (n *Network) StreamWrite(streamID, peerID string, message *protobuf.Message) (error, int32) {
	var bytes int32
	state, ok := n.ConnectionState(peerID)
	if !ok {
		return errors.New("Network.StreamWrite: connection does not exist"), 0
	}
	state.writerMutex.Lock()
	log.Debugf("Network.StreamWrite write msg to %s", n.GetAddrByPeerID(peerID))
	defer func() {
		state.writerMutex.Unlock()
		log.Debugf("Network.StreamWrite write msg to %s done", n.GetAddrByPeerID(peerID))
	}()
	message.MessageNonce = atomic.AddUint64(&state.messageNonce, 1)

	addrInfo, err := ParseAddress(n.Address)
	if err != nil {
		log.Fatal(err)
		return errors.Errorf("Network.StreamWrite parse address,%s", err.Error()), 0
	}

	if addrInfo.Protocol == "tcp" || addrInfo.Protocol == "kcp" {
		tcpConn, _ := state.conn.(net.Conn)
		if tcpConn == nil {
			return errors.Errorf("Network.StreamWrite connection is nil address,%s", n.GetAddrByPeerID(peerID)), 0
		}

		if peer := n.GetPeerClient(peerID); nil != peer {
			peer.StreamSendQueue <- StreamSendItem{
				TcpConn:  tcpConn,
				Write:    state.writer,
				Message:  message,
				PeerID:   peerID,
				StreamID: streamID,
				Mutex:    state.writerMutex,
			}
			log.Debugf("Network.StreamWrite msg(id:%s) write to addr:%s(streamID:%s) has been put into queue",
				message.MessageID, address, streamID)
		} else {
			log.Error("(tcp/kcp) Network.StreamWrite to addr:", n.GetAddrByPeerID(peerID), "err: client does not exist")
		}

		/*
			err, bytes = n.streamSendMessage(tcpConn, state.writer, message, address, streamID)
					if err != nil {
						log.Error("(tcp/kcp) Network.StreamWrite to addr:", address, "err:", err.Error())
					}
		*/
	}
	return err, bytes
}

// Write asynchronously sends a message to a denoted target address.
func (n *Network) Write(peerID string, message *protobuf.Message) error {
	state, ok := n.ConnectionState(peerID)
	if !ok {
		//n.UpdateConnState(address, PEER_UNREACHABLE)
		return errors.New("Network.write: connection does not exist")
	}
	state.writerMutex.Lock()
	log.Debugf("Network.Write write msg to %s", n.GetAddrByPeerID(peerID))
	defer func() {
		state.writerMutex.Unlock()
		log.Debugf("Network.Write write msg to %s done", n.GetAddrByPeerID(peerID))
	}()
	message.MessageNonce = atomic.AddUint64(&state.messageNonce, 1)

	addrInfo, err := ParseAddress(n.Address)
	if err != nil {
		log.Fatal(err)
		return errors.Errorf("Network.Write parse address,%s", err.Error())
	}

	if addrInfo.Protocol == "tcp" || addrInfo.Protocol == "kcp" {
		tcpConn, _ := state.conn.(net.Conn)
		if tcpConn == nil {
			return errors.Errorf("Network.Write connection is nil address,%s", n.GetAddrByPeerID(peerID))
		}
		err = n.sendMessage(tcpConn, state.writer, message, peerID)
		if err != nil {
			log.Error("(tcp/kcp) write to addr:", n.GetAddrByPeerID(peerID), "err:", err.Error())
		}
	}
	if addrInfo.Protocol == "udp" {
		//udpConn, _ := state.conn.(*net.UDPConn)
		//udpConn.SetWriteDeadline(time.Now().Add(n.opts.writeTimeout))
		err = n.sendUDPMessage(state.writer, message, state.writerMutex, state, peerID)
		if err != nil {
			log.Error("(udp) write to addr:", peerID, "err:", err.Error())
		}
	}

	if addrInfo.Protocol == "quic" {
		//quicConn, _ := state.conn.(quic.Stream)
		//quicConn.SetWriteDeadline(time.Now().Add(n.opts.writeTimeout))
		err = n.sendQuicMessage(state.writer, message, state.writerMutex)
		if err != nil {
			log.Error("(quic) write to addr:", n.GetAddrByPeerID(peerID), "err:", err.Error())
		}
	}

	if err != nil {
		log.Errorf("Network.Wirte error:%s, begin to delete client and connection resource from sync.Maps，client addr:%s", err.Error(), n.GetAddrByPeerID(peerID))
		if client := n.GetPeerClient(peerID); client != nil {
			client.Close()
		} else {
			log.Errorf("get client entry err in Writer:%s", err.Error())
		}
	}
	return err
}

// Broadcast asynchronously broadcasts a message to all peer clients.
func (n *Network) Broadcast(ctx context.Context, message proto.Message) {
	signed, err := n.PrepareMessage(ctx, message)
	if err != nil {
		log.Errorf("network: failed to broadcast message")
		return
	}

	n.EachPeer(func(client *PeerClient) bool {
		err := n.Write(client.PeerID(), signed)
		if err != nil {
			log.Warnf("failed to send message to peer %v [err=%s]", client.ID, err)
		}
		return true
	})
}

// Broadcast asynchronously broadcasts a message to all peer clients.
func (n *Network) BroadcastToPeers(ctx context.Context, message proto.Message) {
	signed, err := n.PrepareMessage(ctx, message)
	if err != nil {
		log.Errorf("network: failed to broadcast message")
		return
	}

	n.EachPeer(func(client *PeerClient) bool {
		proxyAddr, _ := n.GetWorkingProxyServer()
		if client.Address == proxyAddr {
			return true
		}

		err := n.Write(client.PeerID(), signed)
		if err != nil {
			log.Warnf("in BroadcastToPeers failed to send message to peer,peer addr:%s, peer.ID:%v, err:%s", client.Address, client.ID, err)
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
	log.Info("begin to release all resource about Network abstract")
	time.Sleep(1 * time.Second) // to avoid other goroutine is using the peer and connection resource

	log.Info("delete all installed component, reset network.Components is NewComponentList")
	n.Components = NewComponentList() //delete installed components
	log.Infof("delete all relevant connections&peers resource, client.len:%d, connection.len:%d", n.PeersNum(), n.ConnsNum())
	n.ConnMgr.peers.Range(func(key, peer interface{}) bool {
		peer.(*PeerClient).Close()
		return true
	})
	close(n.Kill)
}

func (n *Network) PeersNum() int {
	count := 0
	n.ConnMgr.peers.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

func (n *Network) ConnsNum() int {
	count := 0
	n.ConnMgr.connections.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

func (n *Network) EachPeer(fn func(client *PeerClient) bool) {
	n.ConnMgr.peers.Range(func(_, value interface{}) bool {
		client := value.(*PeerClient)
		return fn(client)
	})
}

func (n *Network) SetNetworkID(netID uint32) {
	n.netID = netID
}

func (n *Network) GetNetworkID() uint32 {
	return n.netID
}

func (n *Network) SetProxyServer(proxies []ProxyServer) {
	n.ProxyService.Servers = proxies
}

func (n *Network) EnableProxyMode(enable bool) {
	n.ProxyService.Enable = enable
}

func (n *Network) ProxyModeEnable() bool {
	return n.ProxyService.Enable
}

func (n *Network) GetWorkingProxyServer() (string, string) {
	if n.ProxyModeEnable() == false {
		return "", ""
	}
	return n.ProxyService.Servers[n.ProxyService.WorkID].IP, n.ProxyService.Servers[n.ProxyService.WorkID].PeerID
}

func (n *Network) UpdateProxyWorkID() {
	n.ProxyService.WorkID++
	n.ProxyService.WorkID = n.ProxyService.WorkID % uint16(len(n.ProxyService.Servers))
}

/*func (n *Network) DeletePeerClient(address string) {
	n.peers.Delete(address)
}*/

func (n *Network) FinishProxyServer(protocol string) {
	n.ProxyService.Finish.Range(func(p, notify interface{}) bool {
		if protocol == p.(string) {
			close(notify.(chan struct{}))
		}
		return true
	})
}

func (n *Network) Transports() *sync.Map {
	return n.transports
}

func (n *Network) UpdateConnState(peerID string, state PeerState) {
	n.ConnMgr.connStates.Store(peerID, state)
}

func (n *Network) GetRealConnState(peerID string) (PeerState, error) {
	n.ConnMgr.Mutex.Lock()
	defer n.ConnMgr.Mutex.Unlock()
	state, ok := n.ConnMgr.connStates.Load(peerID)
	if !ok {
		return PEER_UNREACHABLE, errors.Errorf("Network.GetRealConnState connStates does not exist, client addr:%s", n.GetAddrByPeerID(peerID))
	}

	_, ok = n.ConnMgr.peers.Load(peerID)
	if !ok {
		return PEER_UNREACHABLE, errors.Errorf("Network.GetRealConnState peer does not exist, client addr:%s", n.GetAddrByPeerID(peerID))
	}

	_, ok = n.ConnMgr.connections.Load(peerID)
	if !ok {
		return PEER_UNREACHABLE, errors.Errorf("Network.GetRealConnState connection does not exist, client addr:%s", n.GetAddrByPeerID(peerID))
	}

	if state == PEER_REACHABLE {
		return PEER_REACHABLE, nil
	}
	return PEER_UNREACHABLE, errors.Errorf("in Network.GetRealConnState, conn&peer exist but state is UNREACHABLE ,client addr:%s", n.GetAddrByPeerID(peerID))
}

func (n *Network) SetDialTimeout(timeout time.Duration) {
	n.dialTimeout = timeout
}

func (n *Network) SetWrittenBufferSize(bytes int) {
	n.opts.writeBufferSize = bytes
}

func (n *Network) SetReadBufferSize(bytes int) {
	n.opts.recvBufferSize = bytes
}

func (n *Network) EnableCompress() {
	n.compressEnable = true
}

func (n *Network) DisableCompress() {
	n.compressEnable = false
}

func (n *Network) SetCompressAlgo(algo AlgoType) {
	n.compressAlgo = algo
}

func (n *Network) SetCompressFileSize(bytes int) {
	n.CompressCondition.Size = bytes
}

func (n *Network) SetClientID(IP, ID string) {
	n.ClientID.Store(IP, ID)
}

func (n *Network) DisableMsgGoroutine() {
	n.DisableDispatchMsgGoroutine = true
}

func (n *Network) EnableMsgGoroutine() {
	n.DisableDispatchMsgGoroutine = false
}

func (n *Network) SetStreamQueueLen(size uint16) {
	n.streamQueueLen = size
}
