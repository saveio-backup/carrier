/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2020-01-16
 */
package network

import (
	"fmt"
	"sync"

	"github.com/saveio/themis/common/log"
)

type MultiStream struct {
	idIncCounter uint32
	stream       *sync.Map
}

type Stream struct {
	Mutex   *sync.Mutex
	ID      string
	SendCnt uint64
	Client  *PeerClient
}

func (s *Stream) GetSendDataCnt() uint64 {
	return s.SendCnt
}

func NewMultiStream() *MultiStream {
	return &MultiStream{
		idIncCounter: 0,
		stream:       new(sync.Map),
	}
}

func (c *PeerClient) OpenStream() *Stream {
	log.Info("in Network.OpenStream, open stream for addr:", c.Address)
	c.Network.ConnMgr.Lock()
	defer c.Network.ConnMgr.Unlock()

	var stream *MultiStream
	if value, ok := c.Network.ConnMgr.streams.Load(c.PeerID()); ok {
		stream = value.(*MultiStream)
	}

	stream.idIncCounter++

	s := Stream{
		ID:      fmt.Sprintf("%s:%d", c.Address, stream.idIncCounter),
		Mutex:   new(sync.Mutex),
		SendCnt: 0,
		Client:  c,
	}
	if value, ok := c.Network.ConnMgr.streams.Load(c.PeerID()); ok {
		value.(*MultiStream).stream.Store(s.ID, &s)
	} else {
		log.Errorf("New MultiStream maybe does not called. PLEASE CHECK YOUR CODE.")
	}
	return &s
}

func (c *PeerClient) CloseStream(streamID string) {
	if value, ok := c.Network.ConnMgr.streams.Load(c.PeerID()); ok {
		if v, isOK := value.(*MultiStream).stream.Load(streamID); isOK {
			v.(*Stream).Mutex.Lock()
			value.(*MultiStream).stream.Delete(streamID)
			log.Debugf("stream ID:%s has been closed, belong to peer addr:%s", streamID, c.Address)
			v.(*Stream).Mutex.Unlock()
		}
	}
}

func (c *PeerClient) StreamExist(streamID string) bool {
	if value, ok := c.Network.ConnMgr.streams.Load(c.PeerID()); ok {
		if _, isOK := value.(*MultiStream).stream.Load(streamID); isOK {
			return true
		} else {
			return false
		}
	} else {
		return false
	}
}

func (c *PeerClient) Stream(streamID string) *Stream {
	if value, ok := c.Network.ConnMgr.streams.Load(c.PeerID()); ok {
		if v, isOK := value.(*MultiStream).stream.Load(streamID); isOK {
			return v.(*Stream)
		}
	}
	return nil
}
