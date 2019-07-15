package backoff

import (
	"context"
	"sync"
	"time"

	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/carrier/network"

	"github.com/saveio/carrier/network/components/proxy"
	"github.com/saveio/themis/common/log"
)

const (
	defaultComponentInitialDelay    = 5 * time.Second
	defaultComponentMaxAttempts     = 100
	defaultComponentPriority        = 100
	defaultProxyReconnectionTimeout = 3 * time.Second
)

// Component is the backoff Component
type Component struct {
	*network.Component

	// Component options
	// initialDelay specifies initial backoff interval
	initialDelay time.Duration
	// maxAttempts specifies total number of retries
	maxAttempts int
	// priority specifies Component priority
	priority int

	net      *network.Network
	backoffs sync.Map
}

// ComponentOption are configurable options for the backoff Component
type ComponentOption func(*Component)

// WithInitialDelay specifies initial backoff interval
func WithInitialDelay(d time.Duration) ComponentOption {
	return func(o *Component) {
		o.initialDelay = d
	}
}

// WithMaxAttempts specifies max attempts to retry upon client disconnect
func WithMaxAttempts(i int) ComponentOption {
	return func(o *Component) {
		o.maxAttempts = i
	}
}

// WithPriority specifies Component priority
func WithPriority(i int) ComponentOption {
	return func(o *Component) {
		o.priority = i
	}
}

func defaultOptions() ComponentOption {
	return func(o *Component) {
		o.initialDelay = defaultComponentInitialDelay
		o.maxAttempts = defaultComponentMaxAttempts
		o.priority = defaultComponentPriority
	}
}

var (
	_ network.ComponentInterface = (*Component)(nil)
	// ComponentID is used to check existence of the backoff Component
	ComponentID = (*Component)(nil)
)

// New returns a new backoff Component with specified options
func New(opts ...ComponentOption) *Component {
	p := new(Component)
	defaultOptions()(p)

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Startup implements the Component callback
func (p *Component) Startup(net *network.Network) {
	p.net = net
}

// PeerDisconnect implements the Component callback
func (p *Component) PeerDisconnect(client *network.PeerClient) {
	if client.Address == p.net.GetProxyServer() && p.net.ProxyModeEnable() {
		go p.startProxyBackoff(client.Address)
	} else {
		go p.startBackoff(client.Address)
	}
}

// startBackoff uses an exponentially increasing timer to try to reconnect to a given address
func (p *Component) startBackoff(addr string) {
	time.Sleep(p.initialDelay)

	if _, exists := p.backoffs.Load(addr); exists {
		// don't activate if backoff is already active
		log.Infof("backoff skipped for addr %s, already active", addr)
		return
	}
	// reset the backoff counter
	p.backoffs.Store(addr, DefaultBackoff())
	startTime := time.Now()
	for i := 0; i < p.maxAttempts; i++ {
		s, active := p.backoffs.Load(addr)
		if !active {
			break
		}
		b := s.(*Backoff)
		if b.TimeoutExceeded() {
			// check if the backoff expired
			log.Infof("backoff ended for addr %s, timed out after %s", addr, time.Now().Sub(startTime))
			break
		}
		// sleep for a bit before connecting
		d := b.NextDuration()
		log.Infof("backoff reconnecting to %s in %s iteration %d", addr, d, i+1)
		time.Sleep(d)
		if p.net.ConnectionStateExists(addr) {
			// check that the connection is still empty before dialing
			break
		}
		// dial the client and see if it is successful
		c, err := p.net.Client(addr)
		if err != nil {
			continue
		}
		if !p.net.ConnectionStateExists(addr) {
			// check if successfully connected
			continue
		}
		if err := c.Tell(context.Background(), &protobuf.Ping{}); err != nil {
			// ping failed, not really connected
			continue
		}
		// success
		break
	}
	// clean up this backoff
	p.backoffs.Delete(addr)
}

// startBackoff uses an exponentially increasing timer to try to reconnect to a given address
func (p *Component) startProxyBackoff(addr string) {
	time.Sleep(p.initialDelay)

	if _, exists := p.backoffs.Load(addr); exists {
		// don't activate if backoff is already active
		log.Infof("backoff skipped for addr %s, already active", addr)
		return
	}

	addrInfo, err := network.ParseAddress(addr)
	if err != nil {
		log.Error("in proxy backOff, parase address err:", err.Error())
		return
	}
	// reset the backoff counter
	p.backoffs.Store(addr, DefaultBackoff())
	startTime := time.Now()
	for i := 0; i < p.maxAttempts; i++ {
		s, active := p.backoffs.Load(addr)
		if !active {
			break
		}
		b := s.(*Backoff)
		if b.TimeoutExceeded() {
			// check if the backoff expired
			log.Infof("backoff ended for addr %s, timed out after %s", addr, time.Now().Sub(startTime))
			break
		}
		// sleep for a bit before connecting
		d := b.NextDuration()
		log.Infof("backoff reconnecting to %s in %s iteration %d", addr, d, i+1)
		time.Sleep(d)
		if p.net.ConnectionStateExists(addr) {
			// check that the connection is still empty before dialing
			log.Info("in proxy backOff, connection state exists, does not need to reconnect again.")
			break
		}
		p.net.ProxyFinish.Store(addrInfo.Protocol, make(chan struct{}))
		switch addrInfo.Protocol {
		case "tcp":
			proxy.TcpComponentStartup(p.net)
		case "udp":
			proxy.UDPComponentStartup(p.net)
		case "quic":
			proxy.QuicComponentStartup(p.net)
		case "kcp":
			proxy.KCPComponentStartup(p.net)
		default:
			log.Error("proxy backOff only support tcp/udp/kcp/quic.")
			break
		}

		if !p.net.ConnectionStateExists(addr) {
			// check if successfully connected
			continue
		}
		break
	}
	switch addrInfo.Protocol {
	case "tcp":
		p.net.BlockUntilTcpProxyFinish()
	case "udp":
		p.net.BlockUntilUDPProxyFinish()
	case "quic":
		p.net.BlockUntilQuicProxyFinish()
	case "kcp":
		p.net.BlockUntilKCPProxyFinish()
	default:
		log.Error("proxy backOff blocked only support tcp/udp/kcp/quic.")
	}
	// clean up this backoff
	p.backoffs.Delete(addr)
}
