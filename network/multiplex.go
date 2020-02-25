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
	mutex   *sync.Mutex
	ID      string
	SendCnt uint64
	Client  *PeerClient
}

func (s *Stream) GetSendDataCnt() uint64 {
	return s.SendCnt
}

func NewMultiStream() MultiStream {
	return MultiStream{
		idIncCounter: 0,
		stream:       new(sync.Map),
	}
}

func (peer *PeerClient) OpenStream() *Stream {
	log.Info("in Network.OpenStream, open stream for addr:", peer.Address)
	peer.Network.ConnMgr.Lock()
	defer peer.Network.ConnMgr.Unlock()

	var stream MultiStream
	if value, ok := peer.Network.ConnMgr.streams.Load(peer.Address); ok {
		stream = value.(MultiStream)
	}

	stream.idIncCounter++

	s := Stream{
		ID:      fmt.Sprintf("%s:%d", peer.Address, stream.idIncCounter),
		mutex:   new(sync.Mutex),
		SendCnt: 0,
		Client:  peer,
	}
	if value, ok := peer.Network.ConnMgr.streams.Load(peer.Address); ok {
		value.(MultiStream).stream.Store(s.ID, &s)
	} else {
		log.Errorf("New MultiStream maybe does not called. PLEASE CHECK YOUR CODE.")
	}
	return &s
}

func (peer *PeerClient) CloseStream(streamID string) {
	if value, ok := peer.Network.ConnMgr.streams.Load(peer.Address); ok {
		if v, isOK := value.(MultiStream).stream.Load(streamID); isOK {
			v.(*Stream).mutex.Lock()
			value.(MultiStream).stream.Delete(streamID)
			log.Debugf("stream ID:%s has been closed, belong to peer addr:%s", streamID, peer.Address)
			v.(*Stream).mutex.Unlock()
		}
	}
}

func (peer *PeerClient) StreamExist(streamID string) bool {
	if value, ok := peer.Network.ConnMgr.streams.Load(peer.Address); ok {
		if _, isOK := value.(MultiStream).stream.Load(streamID); isOK {
			return true
		} else {
			return false
		}
	} else {
		return false
	}
}
