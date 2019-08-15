package keepalive

import (
	"context"
	"time"

	"sync"

	"errors"

	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/carrier/network"
	"github.com/saveio/themis/common/log"
)

const (
	DefaultKeepaliveInterval      = 3 * time.Second
	DefaultKeepaliveTimeout       = 15 * time.Second
	DefaultProxyKeepaliveInterval = 3 * time.Second
	DefaultProxyKeepaliveTimeout  = 180 * time.Second
)

// Component is the keepalive Component
type Component struct {
	*network.Component

	// interval to send keepalive msg
	proxyKeepaliveInterval time.Duration
	// total keepalive timeout
	proxyKeepaliveTimeout time.Duration

	// interval to send keepalive msg
	keepaliveInterval time.Duration
	// total keepalive timeout
	keepaliveTimeout time.Duration

	// Channel for peer network state change notification
	peerStateChan chan *PeerStateEvent
	stopCh        chan struct{}
	// map to save last state for a peer
	//lastStates map[string]PeerState
	lastStates *sync.Map
	net        *network.Network
}

var stateString = []string{
	"unknown",
	"unreachable",
	"reachable",
	"ready",
}

type PeerStateEvent struct {
	Address string
	State   network.PeerState
}

// ComponentOption are configurable options for the keepalive Component
type ComponentOption func(*Component)

func WithKeepaliveTimeout(t time.Duration) ComponentOption {
	return func(o *Component) {
		o.keepaliveTimeout = t
	}
}

func WithKeepaliveInterval(t time.Duration) ComponentOption {
	return func(o *Component) {
		o.keepaliveInterval = t
	}
}

func WithPeerStateChan(c chan *PeerStateEvent) ComponentOption {
	return func(o *Component) {
		o.peerStateChan = c
	}
}

func defaultOptions() ComponentOption {
	return func(o *Component) {
		o.keepaliveInterval = DefaultKeepaliveInterval
		o.keepaliveTimeout = DefaultKeepaliveTimeout
		o.proxyKeepaliveInterval = DefaultProxyKeepaliveInterval
		o.proxyKeepaliveTimeout = DefaultProxyKeepaliveTimeout
	}
}

var (
	_ network.ComponentInterface = (*Component)(nil)
	// ComponentID is used to check existence of the keepalive Component
	ComponentID = (*Component)(nil)
)

// New returns a new keepalive Component with specified options
func New(opts ...ComponentOption) *Component {
	p := new(Component)
	defaultOptions()(p)

	p.stopCh = make(chan struct{})
	p.lastStates = new(sync.Map)
	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Startup implements the Component callback
func (p *Component) Startup(net *network.Network) {
	p.net = net

	// start keepalive service
	go p.keepaliveService()
	go p.proxyKeepaliveService()
}

func (p *Component) Cleanup(net *network.Network) {
	close(p.stopCh)
}

func (p *Component) PeerConnect(client *network.PeerClient) {
	p.updateLastStateAndNotify(client, network.PEER_REACHABLE)
}

func (p *Component) PeerDisconnect(client *network.PeerClient) {
	p.updateLastStateAndNotify(client, network.PEER_UNREACHABLE)
	//client.Close()
}

func (p *Component) Receive(ctx *network.ComponentContext) error {
	switch ctx.Message().(type) {
	case *protobuf.Keepalive:
		// Send keepalive response to peer.
		err := ctx.Reply(context.Background(), &protobuf.KeepaliveResponse{})
		if err != nil {
			return err
		}
		p.updateLastStateAndNotify(ctx.Client(), network.PEER_REACHABLE)
	case *protobuf.KeepaliveResponse:
		//p.updateLastStateAndNotify(ctx.Client(), network.PEER_REACHABLE)
	case *protobuf.Ping:
		//p.updateLastStateAndNotify(ctx.Client(), network.PEER_REACHABLE)
		err := ctx.Reply(context.Background(), &protobuf.Pong{})
		if err != nil {
			return err
		}
	case *protobuf.Pong:
		p.updateLastStateAndNotify(ctx.Client(), network.PEER_REACHABLE)

	}

	return nil
}

func (p *Component) keepaliveService() {
	t := time.NewTicker(p.keepaliveInterval)

	for {
		select {
		case <-t.C:
			// broadcast keepalive msg to all peers
			p.net.BroadcastToPeers(context.Background(), &protobuf.Keepalive{})
			p.timeout()
		case <-p.stopCh:
			t.Stop()
			break
		}
	}
}

func (p *Component) proxyKeepaliveService() {
	t := time.NewTicker(p.proxyKeepaliveInterval)

	for {
		select {
		case <-t.C:
			// broadcast keepalive msg to all peers
			if p.net.ProxyModeEnable() == false {
				log.Info("proxyModeEnable is false, proxyKeepaliveService groutine exit now")
				return
			}
			client := p.net.GetPeerClient(p.net.GetWorkingProxyServer())
			if client == nil {
				log.Errorf("in proxyKeepliveService, connection to proxy:%s err", p.net.GetWorkingProxyServer())
				continue
			}
			err := client.Tell(context.Background(), &protobuf.Keepalive{})
			if err != nil {
				log.Error("in proxyKeepaliveServer, send Keepalive msg ERROR:", err.Error(), ",working proxy addr:", p.net.GetWorkingProxyServer())
			}
			if time.Now().After(client.Time.Add(p.proxyKeepaliveTimeout)) {
				p.updateLastStateAndNotify(client, network.PEER_UNREACHABLE)
				client.Close()
			}
		case <-p.stopCh:
			t.Stop()
			break
		}
	}
}

// check all connetion if keepalive timeout
func (p *Component) timeout() {
	p.net.EachPeer(func(client *network.PeerClient) bool {
		if client.Address == p.net.GetWorkingProxyServer() {
			return true
		}
		// timeout notify state change
		if time.Now().After(client.Time.Add(p.keepaliveTimeout)) {
			p.updateLastStateAndNotify(client, network.PEER_UNREACHABLE)
			client.Close()
		}
		return true
	})
}

func (p *Component) updateLastStateAndNotify(client *network.PeerClient, state network.PeerState) {
	client.ConnStateMutex.Lock()
	defer client.ConnStateMutex.Unlock()

	if last, ok := p.lastStates.Load(client.Address); ok {
		log.Debugf("[keepalive]address:%s, last status:%d, tobe changed status:%d", client.Address, last, state)
	}
	p.lastStates.Store(client.Address, state)
	p.net.UpdateConnState(client.Address, state)
}

func (p *Component) GetPeerStateByAddress_TODO_Delete(address string) (network.PeerState, error) {
	last, ok := p.lastStates.Load(address)
	if !ok {
		log.Debugf("[keepalive] address:%s, peer status does not exist in p.lastStates Map.", address)
		return network.PEER_UNKNOWN, errors.New("[keepalive]Does not know peer status")
	}
	return last.(network.PeerState), nil
}

func (p *Component) GetPeerStateChan() chan *PeerStateEvent {
	return p.peerStateChan
}

func (p *Component) GetStopChan() chan struct{} {
	return p.stopCh
}
