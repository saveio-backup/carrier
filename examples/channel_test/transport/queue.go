package transport

import (
	"sync"
)

type Node struct {
	data interface{}
	next *Node
}

type Queue struct {
	sync.RWMutex
	head        *Node
	tail        *Node
	length      uint32
	capacity    uint32
	DataCh      chan struct{}
	DeliverChan chan MessageID
}

func NewQueue(cap uint32) *Queue {
	q := &Queue{
		head:        nil,
		tail:        nil,
		capacity:    cap,
		length:      0,
		DataCh:      make(chan struct{}, 1),
		DeliverChan: make(chan MessageID),
	}
	return q
}

func (q *Queue) Len() uint32 {
	q.RLock()
	defer q.RUnlock()

	return q.length
}
func (q *Queue) Push(data interface{}) bool {
	q.Lock()
	defer func() {
		if q.length == 1 {
			q.DataCh <- struct{}{}
		}
		q.Unlock()
	}()

	if q.length == q.capacity {
		return false
	}

	node := &Node{
		data: data,
		next: nil,
	}

	if q.tail == nil {
		q.head = node
		q.tail = node
	} else {
		q.tail.next = node
		q.tail = node
	}
	q.length++
	return true
}

func (q *Queue) Peek() (interface{}, bool) {
	q.RLock()
	defer q.RUnlock()

	if q.head == nil {
		return nil, false
	}
	return q.head.data, true
}

func (q *Queue) Pop() (interface{}, bool) {
	q.Lock()
	defer q.Unlock()

	if q.head == nil {
		return nil, false
	}

	data := q.head.data
	q.head = q.head.next
	if q.head == nil {
		q.tail = nil
	}
	q.length--
	return data, true
}

