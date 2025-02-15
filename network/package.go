/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-01-15
 */
package network

import (
	"io"
	"sync"

	"net"

	"encoding/binary"

	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/carrier/types/opcode"
	"github.com/saveio/themis/common/log"
)

const (
	MAX_PACKAGE_SIZE        = 1024 * 64
	STORE_PACKAGE_REAL_SIZE = 2
)

func (n *Network) BuildRawContent(message *protobuf.Message) []byte {
	bytes, err := proto.Marshal(message)
	if err != nil {
		log.Errorf("package: failed to Marshal entire message, err: %f", err.Error())
		return nil
	}

	buffer := make([]byte, STORE_PACKAGE_REAL_SIZE)
	binary.BigEndian.PutUint16(buffer, uint16(len(bytes)))
	buffer = append(buffer, bytes...)
	if len(buffer) > MAX_PACKAGE_SIZE {
		log.Errorf("Package size bigger than 64k is not allow with UDP protocol.")
		return nil
	}
	return buffer
}

// sendMessage marshals, signs and sends a message over a stream.
func (n *Network) sendUDPMessage(w io.Writer, message *protobuf.Message, writerMutex *sync.Mutex, state *ConnState, address string) error {
	buffer := n.BuildRawContent(message)
	if nil == buffer {
		log.Error("build raw conent from protobuf err in send udp message")
		return nil
	}

	//caller will take lock
	//writerMutex.Lock()
	//defer writerMutex.Unlock()

	udpConn, _ := state.conn.(*net.UDPConn)
	_, err := udpConn.Write(buffer)
	if err != nil {
		log.Errorf("package: failed to write entire message, err: %+v", err)
	}
	return nil
}

// receiveMessage reads, unmarshals and verifies a message from a net.Conn.
func (n *Network) receiveUDPMessage(conn interface{}) (*protobuf.Message, error) {
	var err error
	buffer := make([]byte, MAX_PACKAGE_SIZE)

	udpConn, _ := conn.(*net.UDPConn)
	length, remoteAddr, err := udpConn.ReadFromUDP(buffer)
	if remoteAddr == nil && length == 0 {
		return nil, nil
	}
	size := binary.BigEndian.Uint16(buffer[0:2])
	// Deserialize message.
	msg := new(protobuf.Message)
	err = proto.Unmarshal(buffer[2:2+size], msg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal message")
	}

	if msg.Opcode == uint32(opcode.ProxyResponseCode) || msg.Opcode == uint32(opcode.KeepaliveResponseCode) {
		return msg, nil
	}

	// Check if any of the message headers are invalid or null.
	if msg.Opcode == 0 || msg.Sender == nil || msg.Sender.NetKey == nil || len(msg.Sender.Address) == 0 || msg.NetID != n.GetNetworkID() {
		return nil, errors.New("received an invalid message (either no opcode, no sender, no net key, or no signature,or no NetID) from a peer")
	}

	return msg, nil
}
