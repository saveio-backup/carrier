package server

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/ontio/ontology-eventbus/actor"
	p2pNet "github.com/saveio/carrier/network"
	act "github.com/saveio/pylons/actor/client"
	"github.com/saveio/pylons/test/p2p"
	"github.com/saveio/themis/common/log"
)

type MessageHandler func(msgData interface{}, pid *actor.PID)

type P2PActor struct {
	net         *p2p.Network
	props       *actor.Props
	msgHandlers map[string]MessageHandler
	localPID    *actor.PID
}

func NewP2PActor(n *p2p.Network) (*actor.PID, error) {
	var err error
	p2pActor := &P2PActor{
		net:         n,
		msgHandlers: make(map[string]MessageHandler),
	}
	p2pActor.localPID, err = p2pActor.Start()
	if err != nil {
		return nil, err
	}
	return p2pActor.localPID, nil

}

func (this *P2PActor) Start() (*actor.PID, error) {
	this.props = actor.FromProducer(func() actor.Actor { return this })
	localPid, err := actor.SpawnNamed(this.props, "net_server")
	if err != nil {
		return nil, fmt.Errorf("[P2PActor] start error:%v", err)
	}
	this.localPID = localPid
	return localPid, err
}

func (this *P2PActor) Receive(ctx actor.Context) {
	switch msg := ctx.Message().(type) {
	case *actor.Restarting:
		log.Warn("[oniP2p]actor restarting")
	case *actor.Stopping:
		log.Warn("[oniP2p]actor stopping")
	case *actor.Stopped:
		log.Warn("[oniP2p]actor stopped")
	case *actor.Started:
		log.Debug("[oniP2p]actor started")
	case *actor.Restart:
		log.Warn("[oniP2p]actor restart")
	case *act.ConnectReq:
		err := this.net.Connect(msg.Address)
		ctx.Sender().Request(&act.P2pResp{err}, ctx.Self())
	case *act.CloseReq:
		err := this.net.Close(msg.Address)
		ctx.Sender().Request(&act.P2pResp{err}, ctx.Self())
	case *act.SendReq:
		err := this.net.Send(msg.Data, msg.Address)
		ctx.Sender().Request(&act.P2pResp{err}, ctx.Self())
	default:
		log.Error("[P2PActor] receive unknown message type!")
	}
}

func (this *P2PActor) Broadcast(message proto.Message) {
	ctx := p2pNet.WithSignMessage(context.Background(), false)
	this.net.P2p.Broadcast(ctx, message)
}

func (this *P2PActor) RegMsgHandler(msgName string, handler MessageHandler) {
	this.msgHandlers[msgName] = handler
}

func (this *P2PActor) UnRegMsgHandler(msgName string, handler MessageHandler) {
	delete(this.msgHandlers, msgName)
}

func (this *P2PActor) GetLocalPID() *actor.PID {
	return this.localPID
}
