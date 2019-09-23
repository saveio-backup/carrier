package network

import (
	"encoding/binary"
	"io"
	"sync"
	"time"

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
func (n *Network) sendMessage(tcpConn net.Conn, w io.Writer, message *protobuf.Message, writerMutex *sync.Mutex, address string) error {
	log.Infof("(kcp/tcp)in Network.sendMessage, send from addr:%s, send to:%s, message.opcode:%d, msg.nonce:%d",
		n.ID.Address, address, message.Opcode, message.MessageNonce)
	bytes, err := proto.Marshal(message)
	if err != nil {
		return errors.Wrap(err, "failed to marshal message")
	}
	log.Infof("(kcp/tcp)in Network.sendMessage, marshal message finished, send to:%s, message.opcode:%d, msg.nonce:%d",
		address, message.Opcode, message.MessageNonce)
	msgOriginSize := len(bytes)
	if msgOriginSize == 0 {
		log.Info("stack info:", fmt.Sprintf("%s", debug.Stack()))
		log.Error("in tcp sendMessage,len(message) == 0, write to remote addr:", w.(net.Conn).RemoteAddr())
		return errors.New("tcp sendMessage,len(message) is empty")
	}

	if n.compressEnable && msgOriginSize >= n.CompressCondition.Size {
		bytes, err = n.Compress(bytes)
		if err != nil {
			log.Error("compress enable, however, compress false, algo:", n.compressAlgo, ",err:", err.Error())
			return errors.Errorf("compress err:%s, algo:%s", err.Error(), AlgoName[n.compressAlgo])
		}
	}
	log.Infof("(kcp/tcp)in Network.sendMessage, compress successed. compress enable:%d, compress condition size:%d, origin size:%d, after compress size:%d, "+
		"compress algo:%s, send to:%s, message.opcode:%d, msg.nonce:%d", n.compressEnable, n.CompressCondition.Size, msgOriginSize, len(bytes), AlgoName[n.compressAlgo],
		address, message.Opcode, message.MessageNonce)
	// Serialize size.
	buffer := make([]byte, 10)
	binary.BigEndian.PutUint32(buffer, n.GetNetworkID())
	binary.BigEndian.PutUint16(buffer[4:], uint16(n.GenCompressInfo(msgOriginSize)))
	binary.BigEndian.PutUint32(buffer[6:], uint32(len(bytes)))

	buffer = append(buffer, bytes...)
	//totalSize := len(buffer)

	// Write until all bytes have been written.
	bytesWritten, totalBytesWritten := 0, 0

	//writerMutex.Lock()
	//defer writerMutex.Unlock()
	var blocks int
	blocks = len(buffer)/PER_SEND_BLOCK_SIZE + 1
	tcpConn.SetWriteDeadline(time.Now().Add(time.Duration(n.opts.perBlockWriteTimeout*blocks) * time.Second))
	bw, _ := w.(*bufio.Writer)
	for totalBytesWritten < len(buffer) && err == nil {
		log.Infof("(kcp/tcp)in Network.sendMessage, begin to write socket buffer, send from addr:%s, send to:%s, "+
			"message.opcode:%d, msg.nonce:%d, write buffer size:%d", n.ID.Address, address, message.Opcode, message.MessageNonce, bytesWritten)
		bytesWritten, err = w.Write(buffer[totalBytesWritten:])
		if err != nil {
			log.Errorf("stream: failed to write entire buffer, err: %+v", err)
			break
		}
		log.Infof("(kcp/tcp)in Network.sendMessage, once write buffer successed; send from addr:%s, send to:%s, "+
			"message.opcode:%d, msg.nonce:%d, write buffer size:%d", n.ID.Address, address, message.Opcode, message.MessageNonce, bytesWritten)
		totalBytesWritten += bytesWritten
		if bw.Available() <= 0 {
			if err = bw.Flush(); err != nil {
				log.Error("stream flush err in buffer immediately written:", err.Error())
				break
			}
			log.Infof("(kcp/tcp)in Network.sendMessage, immediately flush successed; send from addr:%s, send to:%s, "+
				"message.opcode:%d, msg.nonce:%d, flush buffer size:%d", n.ID.Address, address, message.Opcode, message.MessageNonce, bw.Size())
		}
	}

	if err != nil {
		return errors.Errorf("stream: failed to write to socket, send from addr:%s, send to:%s, "+
			"message.opcode:%d, msg.nonce:%d, send has written byte:%d, total need to be written:%d, err:%s", n.ID.Address, address, message.Opcode, message.MessageNonce, totalBytesWritten, len(buffer), err.Error())
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	log.Infof("(kcp/tcp)in Network.sendMessage, successed finished; send from addr:%s, send to:%s, message.opcode:%d, msg.nonce:%d", n.ID.Address, address, message.Opcode, message.MessageNonce)
	return nil
}

// receiveMessage reads, unmarshals and verifies a message from a net.Conn.
func (n *Network) receiveMessage(client *PeerClient, conn net.Conn) (*protobuf.Message, error) {
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

	buffer = make([]byte, 2)
	bytesRead, totalBytesRead = 0, 0

	for totalBytesRead < 2 && err == nil {
		bytesRead, err = conn.Read(buffer[totalBytesRead:])
		totalBytesRead += bytesRead
	}

	if err != nil {
		return nil, errors.Errorf("tcp receive invalid message size bytes err:%s", err.Error())
	}

	compressInfo := binary.BigEndian.Uint16(buffer)
	algo, compEnable := n.GetCompressInfo(compressInfo)

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

	// Read until all message bytes have been read.
	buffer = make([]byte, size)

	bytesRead, totalBytesRead = 0, 0

	for totalBytesRead < int(size) && err == nil {
		bytesRead, err = conn.Read(buffer[totalBytesRead:])
		if err == nil && client != nil {
			client.Time = time.Now()
		}
		totalBytesRead += bytesRead
	}

	if err != nil {
		return nil, errors.Errorf("tcp receive invalid message body bytes err:%s, total to be read:%d, has read:%d", err.Error(), size, totalBytesRead)
	}

	if compEnable {
		buffer, err = n.Uncompress(buffer, AlgoType(algo))
		if err != nil {
			log.Error("uncompress buffer msg err, err:", err.Error(), ",algo type:", algo)
			return nil, errors.Errorf("uncompress err:%s,algo:%d", err.Error(), algo)
		}
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
