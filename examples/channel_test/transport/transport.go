package transport

import (
	"fmt"
	"sync"
	"reflect"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/saveio/themis/common/log"
	"github.com/saveio/carrier/examples/channel_test/common/messages"
	"github.com/saveio/carrier/examples/channel_test/p2p"
)

type MessageID uint64
type ChannelID uint64

type Transport struct {
	Network         *p2p.Network
	messageQueues   *sync.Map
	kill            chan struct{}
}

type QueueItem struct {
	message   proto.Message
	messageId MessageID
}

func NewTransport() *Transport {
	tranResp := &Transport{
		kill:                make(chan struct{}),
		messageQueues:       new(sync.Map),
	}
	tranResp.Network = p2p.NewP2P(tranResp)
	return tranResp
}

// messages first be queued, only can be send when Delivered for previous msssage is received
func (this *Transport) SendAsync(target string, msg proto.Message) error {
	var msgID MessageID

	q := this.GetQueue(target)
	switch msg.(type) {
	case *messages.Message:
		msgID = MessageID(msg.(*messages.Message).MessageId.MessageId)
	default:
		log.Error("[SendAsync] Unknown message type to send async: ", reflect.TypeOf(msg).String())
		return fmt.Errorf("Unknown message type to send async ")
	}

	ok := q.Push(&QueueItem{
		message:   msg,
		messageId: msgID,
	})
	if !ok {
		return fmt.Errorf("failed to push to queue")
	}

	return nil
}

func (this *Transport) GetQueue(target string) *Queue {
	q, ok := this.messageQueues.Load(target)

	if !ok {
		q = this.InitQueue(target)
	}

	return q.(*Queue)
}

func (this *Transport) InitQueue(target string) *Queue {
	q := NewQueue(10000)

	this.messageQueues.Store(target, q)

	// queueid cannot be pointer type otherwise it might be updated outside QueueSend
	go this.QueueSend(q, target)

	return q
}

func (this *Transport) QueueSend(queue *Queue, target string) {
	var interval time.Duration = 3

	t := time.NewTimer(interval * time.Second)

	for {
		select {
		case <-queue.DataCh:
			log.Debugf("[QueueSend] <-queue.DataCh Time: %s queue: %p\n", time.Now().String(), queue)
			t.Reset(interval * time.Second)
			this.PeekAndSend(queue, target)
			// handle timeout retry
		case <-t.C:
			log.Debugf("[QueueSend]  <-t.C Time: %s queue: %p\n", time.Now().String(), queue)
			if queue.Len() == 0 {
				continue
			}

			item, _ := queue.Peek()
			msg := item.(*QueueItem).message
			log.Warnf("Timeout retry for msg = %+v\n", msg)

			t.Reset(interval * time.Second)
			err := this.PeekAndSend(queue, target)
			if err != nil {
				log.Error("send message failed:", err)
				t.Stop()
				break
			}
		case msgId := <-queue.DeliverChan:
			log.Debugf("[DeliverChan] Time: %s msgId := <-queue.DeliverChan queue: %p msgId = %+v queue.length: %d\n",
				time.Now().String(), queue, msgId, queue.Len())
			data, _ := queue.Peek()
			if data == nil {
				log.Debug("[DeliverChan] msgId := <-queue.DeliverChan data == nil")
				log.Error("msgId := <-queue.DeliverChan data == nil")
				continue
			}
			item := data.(*QueueItem)
			log.Debugf("[DeliverChan] msgId := <-queue.DeliverChan: %s item = %+v\n",
				reflect.TypeOf(item.message).String(), item.messageId)
			if msgId == item.messageId {

				queue.Pop()
				t.Stop()
				if queue.Len() != 0 {
					log.Debug("msgId.MessageId == item.messageId.MessageId queue.Len() != 0")
					t.Reset(interval * time.Second)
					this.PeekAndSend(queue, target)
				}
			} else {
				log.Debug("[DeliverChan] msgId.MessageId != item.messageId.MessageId queue.Len: ", queue.Len())
				log.Warnf("[DeliverChan] msgId.MessageId: %d != item.messageId.MessageId: %d", msgId,
					item.messageId)
			}
		case <-this.kill:
			log.Info("[QueueSend] msgId := <-this.kill")
			t.Stop()
			break
		}
	}
}

func (this *Transport) PeekAndSend(queue *Queue, target string) error {
	item, ok := queue.Peek()
	if !ok {
		return fmt.Errorf("Error peeking from queue. ")
	}

	msg := item.(*QueueItem).message
	log.Debugf("send msg msg = %+v\n", msg)

	msgId := MessageID(item.(*QueueItem).messageId)
	log.Debugf("[PeekAndSend] msgId: %v, queue: %p\n", msgId, queue)


	if err := this.Network.Send(msg, target); err != nil {
		log.Error("[PeekAndSend] send error: ", err.Error())
		return err
	}

	return nil
}

func (this *Transport) Receive(message proto.Message, from string) {
	log.Debug("[NetComponent] Receive: ", reflect.TypeOf(message).String(), " From: ", from)
	switch message.(type) {
	case *messages.Delivered:
		go this.ReceiveDelivered(message, from)
	default:
		go this.ReceiveMessage(message, from)
	}
}

func (this *Transport) ReceiveMessage(message proto.Message, from string) {
	log.Debugf("[ReceiveMessage] %v from: %v", reflect.TypeOf(message).String(), from)

	var msgID uint64

	switch message.(type) {
	case *messages.Message:
		msg := message.(*messages.Message)
		msgID = msg.MessageId.MessageId
	default:
		log.Warn("[ReceiveMessage] unknown Msg type: ", reflect.TypeOf(message).String())
		return
	}

	log.Debugf("[ReceiveMessage] %v msgId: %v from: %v", reflect.TypeOf(message).String(), msgID, from)
	deliveredMessage := &messages.Delivered{
		MessageId:&messages.MessageID{
			MessageId:msgID,
		},
	}
	//todo: check
	err := this.Network.Send(deliveredMessage, from)
	if err != nil {
		log.Errorf("SendDeliveredMessage (%v) Time: %s DeliveredMessageId: %v  error: %s",
			reflect.TypeOf(message).String(), time.Now().String(), deliveredMessage.MessageId.MessageId, err.Error())
	}
}

func (this *Transport) ReceiveDelivered(message proto.Message, from string) {
	msg := message.(*messages.Delivered)
	msgId := MessageID(msg.MessageId.MessageId)

	log.Infof("[ReceiveDelivered] from: %s msgId: %v\n", from, msgId)
	//todo
	queue, ok := this.messageQueues.Load(from)
	if !ok {
		log.Debugf("[ReceiveDelivered] from: %s Time: %s msgId: %v\n", from, time.Now().String(), msgId)
		log.Error("[ReceiveDelivered] msg.DeliveredMessageId is not in addressQueueMap")
		return
	}

	queue.(*Queue).DeliverChan <- MessageID(msg.MessageId.MessageId)
}
