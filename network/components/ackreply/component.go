package ackreply

import (
	"time"

	"sync"

	"context"

	"math/rand"

	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/carrier/network"
	"github.com/saveio/themis/common/log"
)

const (
	DefaultAckCheckedInterval = 3 * time.Second
	DefaultAckMessageTimeout  = 10 * time.Second
	DefaultDelayResentTime    = 1 * time.Second
)

type stopNotify struct {
	sync.Mutex
	isStop bool
	stopCh chan struct{}
}

// Component is the keepalive Component
type Component struct {
	*network.Component
	// interval to send keepalive msg
	ackCheckedInterval      time.Duration
	ackMessageTimeout       time.Duration
	delayResentTimeDistance time.Duration
	// Channel for peer network state change notification
	peerStateChan chan *PeerStateEvent
	notify        stopNotify
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

func WithAckCheckedInterval(t time.Duration) ComponentOption {
	return func(o *Component) {
		o.ackCheckedInterval = t
	}
}
func WithAckMessageTimeout(t time.Duration) ComponentOption {
	return func(o *Component) {
		o.ackMessageTimeout = t
	}
}

func WithDealyResentTimeDistance(t time.Duration) ComponentOption {
	return func(o *Component) {
		o.delayResentTimeDistance = t
	}
}
func defaultOptions() ComponentOption {
	return func(o *Component) {
		o.ackCheckedInterval = DefaultAckCheckedInterval
		o.ackMessageTimeout = DefaultAckMessageTimeout
		o.delayResentTimeDistance = DefaultDelayResentTime
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

	p.notify.stopCh = make(chan struct{})
	p.notify.isStop = false
	p.lastStates = new(sync.Map)
	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Startup implements the Component callback
func (p *Component) Startup(net *network.Network) {
	p.net = net

}

func (p *Component) Cleanup(net *network.Network) {
	p.notify.Lock()
	defer p.notify.Unlock()
	if p.notify.isStop == true {
		return
	}
	close(p.notify.stopCh)
	p.notify.isStop = true
}

func (p *Component) PeerConnect(client *network.PeerClient) {
	client.EnableAckReply = true
	go p.checkAckReceivedService(client)
}

func (p *Component) PeerDisconnect(client *network.PeerClient) {
	p.notify.Lock()
	defer p.notify.Unlock()
	if p.notify.isStop == true {
		return
	}
	close(p.notify.stopCh)
	p.notify.isStop = true
}

func (p *Component) Receive(ctx *network.ComponentContext) error {
	switch ctx.Message().(type) {
	case *protobuf.AsyncAckResponse:
		msgID := ctx.Message().(*protobuf.AsyncAckResponse).MessageId
		log.Infof("in ackReply component, Ack Message received successed, to-addr:%s, msgID:%s", ctx.Client().Address, msgID)
		ctx.Client().SyncWaitAck.Delete(msgID)
		ctx.Client().AckStatusNotify <- network.AckStatus{MessageID: msgID, Status: network.ACK_SUCCESS}
	}
	return nil
}

func (p *Component) getJitDelayTime(value interface{}) time.Duration {
	delayByFrequency := p.delayResentTimeDistance * time.Duration(value.(*network.PrepareAckMessage).Frequency)
	jitter := time.Millisecond * time.Duration(rand.Intn(200))
	return delayByFrequency + jitter
}

func (p *Component) checkAckReceivedService(client *network.PeerClient) {
	t := time.NewTicker(p.ackCheckedInterval)

	for {
		select {
		case <-t.C:
			// broadcast keepalive msg to all peers
			client.SyncWaitAck.Range(func(key, value interface{}) bool {
				if time.Now().Second()-value.(*network.PrepareAckMessage).WhenSend > int(p.ackMessageTimeout/time.Second) {
					log.Warnf("in ackReply component, Message Timeout and delete now, to-addr:%s, msgID:%s", client.Address, value.(*network.PrepareAckMessage).MessageID)
					client.SyncWaitAck.Delete(value.(*network.PrepareAckMessage).MessageID)
					client.AckStatusNotify <- network.AckStatus{MessageID: value.(*network.PrepareAckMessage).MessageID, Status: network.ACK_FAILED}
					return true
				}

				if time.Now().Second()-value.(*network.PrepareAckMessage).WhenSend < int(p.ackCheckedInterval/time.Second) {
					//avoid resending directly when message put into Map just now
					return true
				}

				if time.Now().Second()-value.(*network.PrepareAckMessage).LatestSendSuccessAt > int(p.ackCheckedInterval/time.Second) {
					go func() {
						time.Sleep(p.getJitDelayTime(value))

						if err := client.Tell(context.Background(), value.(*network.PrepareAckMessage).Message); err != nil {
							log.Errorf("in ackReply component, ReSend Message err:%s", err.Error())
						} else {
							log.Infof("in ackReply component, Resend Message Successed, to-addr:%s, msgID:%s", client.Address, value.(*network.PrepareAckMessage).MessageID)
							value.(*network.PrepareAckMessage).LatestSendSuccessAt = time.Now().Second()
						}
						value.(*network.PrepareAckMessage).Frequency += 1
					}()
				}
				return true
			})
		case <-p.notify.stopCh:
			t.Stop()
			return
		case <-p.net.Kill:
			t.Stop()
			return
		}
	}
}
