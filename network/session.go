/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-05-31
 */
package network

import (
	"bufio"
	"encoding/binary"
	"io"
	"sync"

	"github.com/gogo/protobuf/proto"
	"github.com/lucas-clemente/quic-go"
	"github.com/pkg/errors"
	"github.com/saveio/carrier/internal/protobuf"
	"github.com/saveio/themis/common/log"
)

// sendMessage marshals, signs and sends a message over a stream.
func (n *Network) sendQuicMessage(w io.Writer, message *protobuf.Message, writerMutex *sync.Mutex) error {
	bytes, err := proto.Marshal(message)
	if err != nil {
		return errors.Wrap(err, "failed to marshal message")
	}
	if len(bytes) == 0 {
		log.Error("in tcp sendMessage,len(message) == 0")
		return nil
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

	/*	bw, isBuffered := w.(*bufio.Writer)
		if isBuffered && (bw.Buffered() > 0) && (bw.Available() < totalSize) {
			if err := bw.Flush(); err != nil {
				return err
			}
		}*/

	for totalBytesWritten < len(buffer) && err == nil {
		bytesWritten, err = w.Write(buffer[totalBytesWritten:])
		if err != nil {
			log.Errorf("stream: failed to write entire buffer, err: %+v", err)
		}
		totalBytesWritten += bytesWritten
	}

	if err != nil {
		return errors.Wrap(err, "stream: failed to write to socket")
	}
	bw, _ := w.(*bufio.Writer)
	if err := bw.Flush(); err != nil {
		return err
	}
	return nil
}

// receiveMessage reads, unmarshals and verifies a message from a net.Conn.
func (n *Network) receiveQuicMessage(stream quic.Stream) (*protobuf.Message, error) {
	var err error
	var size uint32
	// Read until all header bytes have been read.
	buffer := make([]byte, 4)
	bytesRead, totalBytesRead := 0, 0

	for totalBytesRead < 4 && err == nil {
		bytesRead, err = io.ReadFull(stream, buffer[totalBytesRead:])
		totalBytesRead += bytesRead
	}
	if binary.BigEndian.Uint32(buffer) != n.GetNetworkID() {
		return nil, errors.Errorf("(quic)receive an invalid message with wrong networkID:%d, expect networkID is:%d", binary.BigEndian.Uint32(buffer), n.GetNetworkID())
	}

	buffer = make([]byte, 4)
	bytesRead, totalBytesRead = 0, 0

	for totalBytesRead < 4 && err == nil {
		//bytesRead, err = stream.Read(buffer[totalBytesRead:])
		bytesRead, err = io.ReadFull(stream, buffer[totalBytesRead:])
		totalBytesRead += bytesRead
	}
	if err != nil {
		return nil, err
	}
	size = binary.BigEndian.Uint32(buffer)
	if size == 0 {
		return nil, errEmptyMsg
	}

	if size > uint32(n.opts.recvBufferSize) {
		return nil, errors.Errorf("(quic)message has length of %d which is either broken or too large(default %d)", size, n.opts.recvBufferSize)
	}

	// Read until all message bytes have been read.
	buffer = make([]byte, size)

	bytesRead, totalBytesRead = 0, 0

	for totalBytesRead < int(size) && err == nil {
		bytesRead, err = io.ReadFull(stream, buffer[totalBytesRead:])
		//bytesRead, err = stream.Read(buffer[totalBytesRead:])

		totalBytesRead += bytesRead
	}

	// Deserialize message.
	msg := new(protobuf.Message)
	err = proto.Unmarshal(buffer, msg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal message")
	}

	// Check if any of the message headers are invalid or null.
	if msg.Opcode == 0 || msg.Sender == nil || msg.Sender.NetKey == nil || len(msg.Sender.Address) == 0 || msg.NetID != n.GetNetworkID() {
		return nil, errors.New("(quic)received an invalid message (either no opcode, no sender, no net key, or no signature) from a peer")
	}
	log.Infof("(quic)in Network.receiveMessage, receive from addr:%s, send to:%s, message.opcode:%d, msg.nonce:%d", msg.Sender.Address, n.ID.Address, msg.Opcode, msg.MessageNonce)
	return msg, nil
}
