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

	"strings"

	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/oniio/oniChain/common/log"
	"github.com/oniio/oniP2p/internal/protobuf"
	"github.com/pkg/errors"
)

const (
	MAX_PACKAGE_SIZE        = 1024 * 64
	STORE_PACKAGE_REAL_SIZE = 2
)

// sendMessage marshals, signs and sends a message over a stream.
func (n *Network) sendUDPMessage(w io.Writer, message *protobuf.Message, writerMutex *sync.Mutex, state *ConnState, address string) error {
	if state.IsDial {
		if dialAddr, ok := n.udpDialAddrs.Load(address); ok {
			message.DialAddress = fmt.Sprintf("udp://%s", dialAddr.(string))
			message.DialAddress = n.Address
		} else {
			log.Errorf("package: failed to load dial address")
		}
	} else {
		message.DialAddress = n.Address
	}

	bytes, err := proto.Marshal(message)
	if err != nil {
		log.Errorf("package: failed to Marshal entire message, err: %f", err.Error())
	}

	buffer := make([]byte, STORE_PACKAGE_REAL_SIZE)
	binary.BigEndian.PutUint16(buffer, uint16(len(bytes)))
	buffer = append(buffer, bytes...)
	if len(buffer) > MAX_PACKAGE_SIZE {
		log.Errorf("Package size bigger than 64k is not allow with UDP protocol.")
	}
	writerMutex.Lock()
	udpConn, ok := state.conn.(*net.UDPConn)
	if !ok {
		log.Errorf("package: failed to write entire message, err: %+v", err)
	}

	if state.IsDial {
		_, err = udpConn.Write(buffer)
	} else {
		index := strings.LastIndex(address, "/")
		resolved, err := net.ResolveUDPAddr("udp", address[index+1:])
		if err != nil {
			return err
		}
		_, err = udpConn.WriteToUDP(buffer, resolved)
	}
	if err != nil {
		log.Errorf("package: failed to write entire message, err: %+v", err)
	}
	writerMutex.Unlock()
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

	if msg.IsProxy == true{
		return msg, nil
	}

	// Check if any of the message headers are invalid or null.
	if msg.Opcode == 0 || msg.Sender == nil || msg.Sender.NetKey == nil || len(msg.Sender.Address) == 0 || msg.NetID != n.GetNetworkID() {
		return nil, errors.New("received an invalid message (either no opcode, no sender, no net key, or no signature,or no NetID) from a peer")
	}

	// Verify signature of message.
	/*	if msg.Signature != nil && !crypto.Verify(
			n.opts.signaturePolicy,
			n.opts.hashPolicy,
			msg.Sender.NetKey,
			SerializeMessage(msg.Sender, msg.Message),
			msg.Signature,
		) {
			return nil, errors.New("received message had an malformed signature")
		}*/

	return msg, nil
}
