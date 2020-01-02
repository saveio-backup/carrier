package transport

import (
	"time"

	"github.com/xtaci/kcp-go"
)

// KCP represents the KCP transport protocol with its respective configurable options.
type KCP struct {
	DataShards     int
	ParityShards   int
	SendWindowSize int
	RecvWindowSize int
}

// NewKCP instantiates a new instance of the KCP protocol.
func NewKCP() *KCP {
	return &KCP{
		DataShards:     0,
		ParityShards:   0,
		SendWindowSize: 10000,
		RecvWindowSize: 10000,
	}
}

// Listen listens for incoming KCP connections on a specified port.
func (t *KCP) Listen(address string) (interface{}, error) {
	listener, err := kcp.ListenWithOptions(address, nil, t.DataShards, t.ParityShards)

	if err != nil {
		return nil, err
	}

	return interface{}(listener), nil
}

// Dial dials an address via. the KCP protocol, with optional Reed-Solomon message sharding.
func (t *KCP) Dial(address string, timeout time.Duration) (interface{}, error) {
	conn, err := kcp.DialWithOptions(address, nil, t.DataShards, t.ParityShards)

	if err != nil {
		return nil, err
	}

	conn.SetWindowSize(t.SendWindowSize, t.RecvWindowSize)

	return interface{}(conn), nil
}
