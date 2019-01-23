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
	"github.com/oniio/oniChain/common/log"
	"github.com/oniio/oniP2p/crypto"
	"github.com/oniio/oniP2p/internal/protobuf"
	"github.com/pkg/errors"
)

// sendMessage marshals, signs and sends a message over a stream.
func (n *Network) sendUDPMessage(w io.Writer, message *protobuf.Message, writerMutex *sync.Mutex, state *ConnState) error {
	bytes, err := proto.Marshal(message)
	if err != nil {
		log.Errorf("package: failed to Marshal entire message, err: %f\n", err.Error())
	}

	buffer := make([]byte, 2)
	binary.BigEndian.PutUint16(buffer, uint16(len(bytes)))
	buffer = append(buffer, bytes...)

	writerMutex.Lock()
	if udpConn, ok := state.conn.(*net.UDPConn); ok {
		/*		udpAddr, err := net.ResolveUDPAddr("udp", address)
				udpAddr = udpAddr
				if err != nil {
					return err
				}*/
		//_, err = udpConn.WriteToUDP(buffer, udpAddr)
		_, err = udpConn.Write(buffer)
		if err != nil {
			log.Errorf("package: failed to write entire message, err: %+v\n", err)
		}
	}
	writerMutex.Unlock()
	return nil
}

// receiveMessage reads, unmarshals and verifies a message from a net.Conn.
func (n *Network) receiveUDPMessage(conn interface{}) (*protobuf.Message, error) {
	var err error
	buffer := make([]byte, 1024*64)

	udpConn, _ := conn.(*net.UDPConn)
	_, err = udpConn.Read(buffer)

	size := binary.BigEndian.Uint16(buffer[0:2])
	// Deserialize message.
	msg := new(protobuf.Message)
	err = proto.Unmarshal(buffer[2:2+size], msg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal message")
	}

	// Check if any of the message headers are invalid or null.
	if msg.Opcode == 0 || msg.Sender == nil || msg.Sender.NetKey == nil || len(msg.Sender.Address) == 0 {
		return nil, errors.New("received an invalid message (either no opcode, no sender, no net key, or no signature) from a peer")
	}

	// Verify signature of message.
	if msg.Signature != nil && !crypto.Verify(
		n.opts.signaturePolicy,
		n.opts.hashPolicy,
		msg.Sender.NetKey,
		SerializeMessage(msg.Sender, msg.Message),
		msg.Signature,
	) {
		return nil, errors.New("received message had an malformed signature")
	}

	return msg, nil
}
