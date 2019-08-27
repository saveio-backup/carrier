package network

import (
	"encoding/binary"
	"io"
	"sync"

	"net"

	"fmt"
	"runtime/debug"

	"bufio"

	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/themis/common/log"
)

var errEmptyMsg = errors.New("received an empty message from a peer")

// sendMessage marshals, signs and sends a message over a stream.
func (n *Network) sendMessage(w io.Writer, message *protobuf.Message, writerMutex *sync.Mutex, address string) error {
	log.Infof("(kcp/tcp)in Network.sendMessage, send from addr:%s, send to:%s, message.opcode:%d, msg.nonce:%d", n.ID.Address, address, message.Opcode, message.MessageNonce)
	bytes, err := proto.Marshal(message)
	if err != nil {
		return errors.Wrap(err, "failed to marshal message")
	}
	if len(bytes) == 0 {
		log.Info("stack info:", fmt.Sprintf("%s", debug.Stack()))
		log.Error("in tcp sendMessage,len(message) == 0, write to remote addr:", w.(net.Conn).RemoteAddr())
		return errors.New("tcp sendMessage,len(message) is empty")
	}

	// Serialize size.
	buffer := make([]byte, 8)
	binary.BigEndian.PutUint32(buffer, n.GetNetworkID())
	binary.BigEndian.PutUint32(buffer[4:], uint32(len(bytes)))

	buffer = append(buffer, bytes...)
	//totalSize := len(buffer)

	// Write until all bytes have been written.
	bytesWritten, totalBytesWritten := 0, 0

	writerMutex.Lock()
	defer writerMutex.Unlock()

	bw, _ := w.(*bufio.Writer)
	for totalBytesWritten < len(buffer) && err == nil {
		bytesWritten, err = w.Write(buffer[totalBytesWritten:])
		if err != nil {
			log.Errorf("stream: failed to write entire buffer, err: %+v", err)
			break
		}
		totalBytesWritten += bytesWritten
		if bw.Available() <= 0 {
			if err = bw.Flush(); err != nil {
				log.Error("stream flush err in buffer immediately written:", err.Error())
				break
			}
		}
	}

	if err != nil {
		return errors.Wrap(err, "stream: failed to write to socket")
	}
	if err := bw.Flush(); err != nil {
		return err
	}

	return nil
}

// receiveMessage reads, unmarshals and verifies a message from a net.Conn.
func (n *Network) receiveMessage(conn net.Conn) (*protobuf.Message, error) {
	var err error
	var size uint32
	// Read until all header bytes have been read.

	buffer := make([]byte, 4)
	bytesRead, totalBytesRead := 0, 0

	for totalBytesRead < 4 && err == nil {
		bytesRead, err = conn.Read(buffer[totalBytesRead:])
		totalBytesRead += bytesRead
	}
	if err != nil {
		return nil, errors.Errorf("tcp receive networkID ahead bytes err:%s", err.Error())
	}
	if binary.BigEndian.Uint32(buffer) != n.GetNetworkID() {
		return nil, errors.Errorf("(tcp)receive an invalid message with wrong networkID:%d, expect networkID is:%d", binary.BigEndian.Uint32(buffer), n.GetNetworkID())
	}

	buffer = make([]byte, 4)
	bytesRead, totalBytesRead = 0, 0

	for totalBytesRead < 4 && err == nil {
		bytesRead, err = conn.Read(buffer[totalBytesRead:])
		totalBytesRead += bytesRead
	}

	if err != nil {
		return nil, errors.Errorf("tcp receive invalid message size bytes err:%s", err.Error())
	}

	size = binary.BigEndian.Uint32(buffer)

	if size == 0 {
		return nil, errEmptyMsg
	}

	if size > uint32(n.opts.recvBufferSize) {
		log.Warnf("(tcp)message has length of %d which is either broken or too large(default %d), please check.", size, n.opts.recvBufferSize)
	}

	// Read until all message bytes have been read.
	buffer = make([]byte, size)

	bytesRead, totalBytesRead = 0, 0

	for totalBytesRead < int(size) && err == nil {
		bytesRead, err = conn.Read(buffer[totalBytesRead:])
		totalBytesRead += bytesRead
	}

	if err != nil {
		return nil, errors.Errorf("tcp receive invalid message body bytes err:%s, total to be read:%d, has read:%d", err.Error(), size, totalBytesRead)
	}

	// Deserialize message.
	msg := new(protobuf.Message)
	err = proto.Unmarshal(buffer, msg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal message")
	}

	// Check if any of the message headers are invalid or null.
	if msg.Opcode == 0 || msg.Sender == nil || msg.Sender.NetKey == nil || len(msg.Sender.Address) == 0 || msg.NetID != n.GetNetworkID() {
		return nil, errors.New("(tcp)received an invalid message (either no opcode, no sender, no net key, or no signature) from a peer")
	}

	log.Infof("(kcp/tcp)in Network.receiveMessage, receive from addr:%s, send to:%s, message.opcode:%d, msg.nonce:%d", msg.Sender.Address, n.ID.Address, msg.Opcode, msg.MessageNonce)

	return msg, nil
}
