package network

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/carrier/peer"

	"runtime/debug"

	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/saveio/themis/common/log"
)

const REQUEST_REPLY_TIMEOUT = "client sync request reply timeout"
const DEFAULT_ACK_REPLY_CAPACITY = 4096

type AckState int

const (
	ACK_SUCCESS AckState = 1
	ACK_FAILED  AckState = 2
)

type PrepareAckMessage struct {
	MessageID string
	Message   proto.Message
	Latest    int
	Frequency uint8
}

type AckStatus struct {
	MessageID string
	Status    AckState
}

// PeerClient represents a single incoming peers client.
type PeerClient struct {
	Network *Network

	ID      *peer.ID
	Address string

	Requests     sync.Map // uint64 -> *RequestState
	RequestNonce uint64

	stream StreamState

	outgoingReady chan struct{}
	incomingReady chan struct{}

	jobs chan func()

	closed         uint32 // for atomic ops
	CloseSignal    chan struct{}
	Time           time.Time
	RecvWindow     *RecvWindow
	enableBackoff  bool
	ConnStateMutex *sync.Mutex

	EnableAckReply  bool
	SyncWaitAck     sync.Map
	AckStatusNotify chan AckStatus
}

// StreamState represents a stream.
type StreamState struct {
	sync.Mutex
	buffer        []byte
	buffered      chan struct{}
	isClosed      bool
	readDeadline  time.Time
	writeDeadline time.Time
}

// RequestState represents a state of a request.
type RequestState struct {
	data        chan proto.Message
	closeSignal chan struct{}
}

// createPeerClient creates a stub peer client.
func createPeerClient(network *Network, address string) (*PeerClient, error) {
	// Ensure the address is valid.
	if _, err := ParseAddress(address); err != nil {
		return nil, err
	}

	client := &PeerClient{
		Network:      network,
		Address:      address,
		RequestNonce: 0,

		incomingReady: make(chan struct{}),
		outgoingReady: make(chan struct{}),

		stream: StreamState{
			buffer:   make([]byte, 0),
			buffered: make(chan struct{}),
		},

		jobs:            make(chan func(), 128),
		CloseSignal:     make(chan struct{}),
		Time:            time.Now(),
		RecvWindow:      NewRecvWindow(network.opts.recvWindowSize),
		enableBackoff:   true,
		EnableAckReply:  false,
		ConnStateMutex:  new(sync.Mutex),
		AckStatusNotify: make(chan AckStatus, DEFAULT_ACK_REPLY_CAPACITY),
	}

	return client, nil
}

// Init initialize a client's Component and starts executing a jobs.
func (c *PeerClient) Init() {
	c.Network.Components.Each(func(Component ComponentInterface) {
		Component.PeerConnect(c)
	})
	go c.executeJobs()
}

func (c *PeerClient) executeJobs() {
	for {
		select {
		case job := <-c.jobs:
			job()
		case <-c.CloseSignal:
			return
		}
	}
}

// Submit adds a job to the execution queue.
func (c *PeerClient) Submit(job func()) {
	select {
	case c.jobs <- job:
	case <-c.CloseSignal:
	}
}

func (c *PeerClient) RemoveEntries() error {
	// Remove entries from node's network.
	clientAddr := c.Address
	if c.ID != nil {
		clientAddr = c.ID.Address
	}
	log.Debugf("remove entries for %s, c.ID: %v", clientAddr, c.ID)
	if len(clientAddr) == 0 {
		return nil
	}

	// close out connections
	if state, ok := c.Network.ConnectionState(clientAddr); ok {
		addrInfo, err := ParseAddress(c.Network.Address)
		if err != nil {
			return err
		}
		if addrInfo.Protocol == "tcp" || addrInfo.Protocol == "kcp" {
			state.conn.(net.Conn).Close()
		}
		if addrInfo.Protocol == "udp" {
			state.conn.(*net.UDPConn).Close()
		}
		if addrInfo.Protocol == "quic" {
			state.conn.(quic.Stream).Close()
		}
		log.Debugf("remove entries of address: %s", clientAddr)
		c.Network.peers.Delete(clientAddr)
		c.Network.connections.Delete(clientAddr)
		c.Network.UpdateConnState(clientAddr, PEER_UNREACHABLE)
		state.conn = nil
		debug.FreeOSMemory()
	}
	return nil
}

// Close stops all sessions/streams and cleans up the nodes in routing table.
func (c *PeerClient) Close() error {
	if atomic.SwapUint32(&c.closed, 1) == 1 {
		return nil
	}
	log.Debugf("close peer: %s, %p", c.Address, c)

	close(c.CloseSignal)

	c.stream.Lock()
	c.stream.isClosed = true
	c.stream.Unlock()

	if err := c.RemoveEntries(); err != nil {
		log.Error("in Peer Close(), RemoveEntries err:", err.Error())
		return err
	}
	c.Network.Components.Each(func(Component ComponentInterface) {
		Component.PeerDisconnect(c)
	})
	return nil
}

// Tell will asynchronously emit a message to a given peer.
func (c *PeerClient) Tell(ctx context.Context, message proto.Message) error {
	signed, err := c.Network.PrepareMessage(ctx, message)
	if err != nil {
		return errors.Wrap(err, "failed to sign message")
	}

	err = c.Network.Write(c.Address, signed)
	if err != nil {
		return errors.Wrapf(err, "failed to send message to %s", c.Address)
	}

	return nil
}

// Request requests for a response for a request sent to a given peer.
func (c *PeerClient) Request(ctx context.Context, req proto.Message, timeout time.Duration) (proto.Message, error) {
	if ctx == nil {
		return nil, errors.New("network: invalid context")
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	signed, err := c.Network.PrepareMessage(ctx, req)
	if err != nil {
		return nil, err
	}

	signed.RequestNonce = atomic.AddUint64(&c.RequestNonce, 1)

	// Start tracking the request.
	channel := make(chan proto.Message, 1)
	closeSignal := make(chan struct{})

	c.Requests.Store(signed.RequestNonce, &RequestState{
		data:        channel,
		closeSignal: closeSignal,
	})

	// Stop tracking the request.
	defer close(closeSignal)
	defer c.Requests.Delete(signed.RequestNonce)

	err = c.Network.Write(c.Address, signed)
	if err != nil {
		return nil, err
	}
	t := time.NewTicker(timeout)
	defer t.Stop()

	select {
	case <-t.C:
		return nil, errors.New(REQUEST_REPLY_TIMEOUT)
	case res := <-channel:
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (c *PeerClient) waitAckLen() int {
	count := 0
	c.SyncWaitAck.Range(func(_, _ interface{}) bool {
		count += 1
		return true
	})
	return count
}

// Request requests for a response for a request sent to a given peer.
func (c *PeerClient) AsyncSendWithAck(ctx context.Context, req proto.Message, msgID string) error {
	if ctx == nil {
		return errors.New("network: invalid context")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if c.waitAckLen() >= DEFAULT_ACK_REPLY_CAPACITY {
		log.Errorf("async send map has been filled fully. length is:%d, send to address:%s, messageID:%s", DEFAULT_ACK_REPLY_CAPACITY, c.Address, msgID)
		return errors.New(fmt.Sprintf("async send map filled fully. cap:%d", DEFAULT_ACK_REPLY_CAPACITY))
	}

	if _, ok := c.SyncWaitAck.Load(msgID); ok {
		return errors.New(fmt.Sprintf("msgID:%s has been in sending queue", msgID))
	}

	signed, err := c.Network.PrepareMessageWithMsgID(ctx, req, msgID)
	if err != nil {
		return err
	}
	c.SyncWaitAck.Store(msgID, &PrepareAckMessage{
		Message:   req,
		Frequency: 1,
		Latest:    time.Now().Second(),
	})

	err = c.Network.Write(c.Address, signed)
	if err != nil {
		return err
	}
	return nil
}

// Reply is equivalent to Write() with an appended nonce to signal a reply.
func (c *PeerClient) Reply(ctx context.Context, nonce uint64, message proto.Message) error {
	msg, err := c.Network.PrepareMessage(ctx, message)
	if err != nil {
		return err
	}

	// Set the nonce.
	msg.RequestNonce = nonce
	msg.ReplyFlag = true

	err = c.Network.Write(c.Address, msg)
	if err != nil {
		return err
	}

	return nil
}

func (c *PeerClient) handleBytes(pkt []byte) {
	c.stream.Lock()
	empty := len(c.stream.buffer) == 0
	c.stream.buffer = append(c.stream.buffer, pkt...)
	c.stream.Unlock()

	if empty {
		select {
		case c.stream.buffered <- struct{}{}:
		default:
		}
	}
}

// Read implement net.Conn by reading packets of bytes over a stream.
func (c *PeerClient) Read(out []byte) (int, error) {
	for {
		c.stream.Lock()
		isClosed := c.stream.isClosed
		n := copy(out, c.stream.buffer)
		c.stream.buffer = c.stream.buffer[n:]
		readDeadline := c.stream.readDeadline
		c.stream.Unlock()

		if isClosed {
			return n, errors.New("closed")
		}
		if !readDeadline.IsZero() && time.Now().After(readDeadline) {
			return n, errors.New("read deadline exceeded")
		}

		if n != 0 {
			return n, nil
		}
		select {
		case <-c.stream.buffered:
		case <-time.After(1 * time.Second):
		}
	}
}

// Write implements net.Conn and sends packets of bytes over a stream.
func (c *PeerClient) Write(data []byte) (int, error) {
	c.stream.Lock()
	writeDeadline := c.stream.writeDeadline
	c.stream.Unlock()

	if !writeDeadline.IsZero() && time.Now().After(writeDeadline) {
		return 0, errors.New("write deadline exceeded")
	}

	ctx := WithSignMessage(context.Background(), true)
	err := c.Tell(ctx, &protobuf.Bytes{Data: data})
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

// LocalAddr implements net.Conn.
func (c *PeerClient) LocalAddr() net.Addr {
	addr, err := ParseAddress(c.Network.Address)
	if err != nil {
		panic(err) // should never happen
	}
	return addr
}

// RemoteAddr implements net.Conn.
func (c *PeerClient) RemoteAddr() net.Addr {
	addr, err := ParseAddress(c.Address)
	if err != nil {
		panic(err) // should never happen
	}
	return addr
}

// SetDeadline implements net.Conn.
func (c *PeerClient) SetDeadline(t time.Time) error {
	c.stream.Lock()
	c.stream.readDeadline = t
	c.stream.writeDeadline = t
	c.stream.Unlock()
	return nil
}

// SetReadDeadline implements net.Conn.
func (c *PeerClient) SetReadDeadline(t time.Time) error {
	c.stream.Lock()
	c.stream.readDeadline = t
	c.stream.Unlock()
	return nil
}

// SetWriteDeadline implements net.Conn.
func (c *PeerClient) SetWriteDeadline(t time.Time) error {
	c.stream.Lock()
	c.stream.writeDeadline = t
	c.stream.Unlock()
	return nil
}

// setIncomingReady sets a client state to ready for the incomming requests.
func (c *PeerClient) setIncomingReady() {
	close(c.incomingReady)
}

// setOutgoingReady sets a client state to ready for the outgoing requests.
func (c *PeerClient) setOutgoingReady() {
	close(c.outgoingReady)
}

// IsIncomingReady returns true if the client has both incoming and outgoing sockets established.
func (c *PeerClient) IsIncomingReady() bool {
	select {
	case <-c.incomingReady:
		return true
	case <-time.After(1 * time.Second):
		return false
	}
}

// IsOutgoingReady returns true if the client has an outgoing socket established.
func (c *PeerClient) IsOutgoingReady() bool {
	select {
	case <-c.outgoingReady:
		return true
	case <-time.After(1 * time.Second):
		return false
	}
}

func (c *PeerClient) ResetConnectionID(cid []byte) {
	c.ID.ConnectionId = cid
}

func (c *PeerClient) DisableBackoff() {
	c.enableBackoff = false
}

func (c *PeerClient) EnableBackoff() {
	c.enableBackoff = true
}

func (c *PeerClient) GetBackoffStatus() bool {
	return c.enableBackoff
}
