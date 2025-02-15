package proxyKeepalive

import (
	"context"
	"time"

	"sync"

	"github.com/pkg/errors"
	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/carrier/network"
	"github.com/saveio/themis/common/log"
)

const (
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

	// Channel for peer network state change notification
	peerStateChan chan *PeerStateEvent
	stopCh        chan struct{}
	// map to save last state for a peer
	//lastStates map[string]PeerState
	lastStates *sync.Map
	net        *network.Network
}

type PeerStateEvent struct {
	Address string
	State   network.PeerState
}

// ComponentOption are configurable options for the keepalive Component
type ComponentOption func(*Component)

func WithPeerStateChan(c chan *PeerStateEvent) ComponentOption {
	return func(o *Component) {
		o.peerStateChan = c
	}
}

func defaultOptions() ComponentOption {
	return func(o *Component) {
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
	//go p.keepaliveService()
	go p.proxyKeepaliveService()
}

func (p *Component) Cleanup(net *network.Network) {
	close(p.stopCh)
}

func (p *Component) PeerConnect(client *network.PeerClient) {
	//p.net.UpdateConnState(client.Address, network.PEER_REACHABLE)
}

func (p *Component) PeerDisconnect(client *network.PeerClient) {
	//p.net.UpdateConnState(client.Address, network.PEER_UNREACHABLE)
}

func (p *Component) Receive(ctx *network.ComponentContext) error {
	if srv, _ := ctx.Network().GetWorkingProxyServer(); srv != ctx.Sender().Address {
		return nil
	}
	switch ctx.Message().(type) {
	case *protobuf.Keepalive:
		// Send keepalive response to peer.
		err := ctx.Client().Tell(context.Background(), &protobuf.KeepaliveResponse{})
		if err != nil {
			return errors.New("in keepalive component send keepalive rsponse err, client.addr:" + ctx.Client().ID.Address)
		}
		//p.net.ConnMgr.Lock()
		//p.net.UpdateConnState(ctx.Client().Address, network.PEER_REACHABLE)
		//p.net.ConnMgr.Unlock()
	case *protobuf.KeepaliveResponse:

	case *protobuf.Ping:
		err := ctx.Client().Tell(context.Background(), &protobuf.Pong{})
		if err != nil {
			return err
		}
	case *protobuf.Pong:
		//p.net.ConnMgr.Lock()
		//p.net.UpdateConnState(ctx.Client().Address, network.PEER_REACHABLE)
		//p.net.ConnMgr.Unlock()
	}

	return nil
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
			proxyServer, peerID := p.net.GetWorkingProxyServer()
			client := p.net.GetPeerClient(peerID)
			if client == nil {
				log.Errorf("in proxyKeepliveService, connection to proxy:%s err, client is nil", proxyServer)
				return
			}
			err := client.Tell(context.Background(), &protobuf.Keepalive{})
			if err != nil {
				log.Error("in proxyKeepaliveServer, send Keepalive msg ERROR:", err.Error(), ",working proxy addr:", proxyServer)
			}
			if time.Now().After(client.Time.Add(p.proxyKeepaliveTimeout)) {
				//p.net.ConnMgr.Lock()
				//It is not need to update connection status ,beacause client.Close() will do!
				//p.net.UpdateConnState(client.Address, network.PEER_UNREACHABLE)
				//p.net.ConnMgr.Unlock()
				//client.Close()
				log.Warnf("expect proxy keepalive response from :%s timeout, local addr is:%s, "+
					"keepaliveTimeout:%d, keepaliveInterval:%d", client.Address, p.net.Address, p.proxyKeepaliveTimeout, p.proxyKeepaliveInterval)
				return
			}
		case <-p.stopCh:
			t.Stop()
			return
		case <-p.net.Kill:
			return
		}
	}
}

func (p *Component) GetPeerStateChan() chan *PeerStateEvent {
	return p.peerStateChan
}

func (p *Component) GetStopChan() chan struct{} {
	return p.stopCh
}
