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
		return errors.Wrap(err, "failed to marshal quic message")
	}
	msgOriginSize := len(bytes)
	if msgOriginSize == 0 {
		log.Error("in quic sendMessage,len(message) == 0")
		return nil
	}

	if n.compressEnable && msgOriginSize >= n.CompressCondition.Size {
		bytes, err = n.Compress(bytes)
		if err != nil {
			log.Error("quic compress enable, however, compress false, algo:", n.compressAlgo, ",err:", err.Error())
			return errors.Errorf("quic compress err:%s, algo:%d", err.Error(), n.compressAlgo)
		}
	}
	// Serialize size.
	buffer := make([]byte, 10)
	binary.BigEndian.PutUint32(buffer, n.GetNetworkID())
	binary.BigEndian.PutUint16(buffer[4:], uint16(n.GenCompressInfo(msgOriginSize)))
	binary.BigEndian.PutUint32(buffer[6:], uint32(len(bytes)))

	buffer = append(buffer, bytes...)

	// Write until all bytes have been written.
	bytesWritten, totalBytesWritten := 0, 0

	writerMutex.Lock()
	defer writerMutex.Unlock()

	bw, _ := w.(*bufio.Writer)
	for totalBytesWritten < len(buffer) && err == nil {
		bytesWritten, err = w.Write(buffer[totalBytesWritten:])
		if err != nil {
			log.Errorf("quic stream: failed to write entire buffer, err: %+v", err)
			break
		}
		totalBytesWritten += bytesWritten
		if bw.Available() <= 0 {
			if err = bw.Flush(); err != nil {
				log.Error("quic stream flush err in buffer immediately written:", err.Error())
				break
			}
		}
	}

	if err != nil {
		return errors.Errorf("quic stream: failed to write to socket, has written byte:%d, total need to be writed:%d, err:%s", totalBytesWritten, len(buffer), err.Error())
	}
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
	if err != nil {
		return nil, errors.Errorf("quic receive networkID bytes err:%s", err.Error())
	}
	if binary.BigEndian.Uint32(buffer) != n.GetNetworkID() {
		return nil, errors.Errorf("(quic)receive an invalid message with wrong networkID:%d, expect networkID is:%d", binary.BigEndian.Uint32(buffer), n.GetNetworkID())
	}

	buffer = make([]byte, 2)
	bytesRead, totalBytesRead = 0, 0

	for totalBytesRead < 2 && err == nil {
		bytesRead, err = io.ReadFull(stream, buffer[totalBytesRead:])
		totalBytesRead += bytesRead
	}

	if err != nil {
		return nil, errors.Errorf("quic receive invalid message size bytes err:%s", err.Error())
	}

	compressInfo := binary.BigEndian.Uint16(buffer)
	algo, compEnable := n.GetCompressInfo(compressInfo)

	buffer = make([]byte, 4)
	bytesRead, totalBytesRead = 0, 0

	for totalBytesRead < 4 && err == nil {
		//bytesRead, err = stream.Read(buffer[totalBytesRead:])
		bytesRead, err = io.ReadFull(stream, buffer[totalBytesRead:])
		totalBytesRead += bytesRead
	}
	if err != nil {
		return nil, errors.Errorf("quic receive invalid message size bytes err:%s", err.Error())
	}
	size = binary.BigEndian.Uint32(buffer)
	if size == 0 {
		return nil, errEmptyMsg
	}

	// Read until all message bytes have been read.
	buffer = make([]byte, size)

	bytesRead, totalBytesRead = 0, 0

	for totalBytesRead < int(size) && err == nil {
		bytesRead, err = io.ReadFull(stream, buffer[totalBytesRead:])
		totalBytesRead += bytesRead
	}

	if err != nil {
		return nil, errors.Errorf("quic receive invalid message body bytes err:%s, total to be read:%d, has read:%d", err.Error(), size, totalBytesRead)
	}

	if compEnable {
		buffer, err = n.Uncompress(buffer, AlgoType(algo))
		if err != nil {
			log.Error("quic uncompress buffer msg err, err:", err.Error(), ",algo type:", algo)
			return nil, errors.Errorf("quic uncompress err:%s,algo:%d", err.Error(), algo)
		}
	}

	// Deserialize message.
	msg := new(protobuf.Message)
	err = proto.Unmarshal(buffer, msg)
	if err != nil {
		return nil, errors.Wrap(err, "quic failed to unmarshal message")
	}

	// Check if any of the message headers are invalid or null.
	if msg.Opcode == 0 || msg.Sender == nil || msg.Sender.NetKey == nil || len(msg.Sender.Address) == 0 || msg.NetID != n.GetNetworkID() {
		return nil, errors.New("(quic)received an invalid message (either no opcode, no sender, no net key, or no signature) from a peer")
	}
	log.Infof("(quic)in Network.receiveMessage, receive from addr:%s, send to:%s, message.opcode:%d, msg.nonce:%d", msg.Sender.Address, n.ID.Address, msg.Opcode, msg.MessageNonce)
	return msg, nil
}
