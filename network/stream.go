package network

import (
	"bufio"
	"encoding/binary"
	"io"
	"sync"

	"net"

	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/oniio/oniChain/common/log"
	"github.com/oniio/oniP2p/crypto"
	"github.com/oniio/oniP2p/internal/protobuf"
	"github.com/pkg/errors"
)

var errEmptyMsg = errors.New("received an empty message from a peer")

// sendMessage marshals, signs and sends a message over a stream.
func (n *Network) sendMessage(w io.Writer, message *protobuf.Message, writerMutex *sync.Mutex, state *ConnState) error {
	bytes, err := proto.Marshal(message)
	if err != nil {
		return errors.Wrap(err, "failed to marshal message")
	}

	// Serialize size.
	buffer := make([]byte, 4)
	binary.BigEndian.PutUint32(buffer, uint32(len(bytes)))

	buffer = append(buffer, bytes...)
	totalSize := len(buffer)

	// Write until all bytes have been written.
	bytesWritten, totalBytesWritten := 0, 0

	writerMutex.Lock()
	fmt.Println("Send Message Value(byte): ", buffer)
	bw, isBuffered := w.(*bufio.Writer)
	if isBuffered && (bw.Buffered() > 0) && (bw.Available() < totalSize) {
		if err := bw.Flush(); err != nil {
			return err
		}
	}

	for totalBytesWritten < len(buffer) && err == nil {
		//bytesWritten, err = w.Write(buffer[totalBytesWritten:])

		if _, ok := state.conn.(*net.TCPConn); ok {
			bytesWritten, err = w.Write(buffer[totalBytesWritten:])
		}

		if udpConn, ok := state.conn.(*net.UDPConn); ok {
			/*			resolved, err := net.ResolveUDPAddr("udp", udpConn.RemoteAddr().String())
						if err != nil {
							return err
						}*/
			fmt.Println("UDPConn begin to wirte:", buffer[totalBytesWritten:])
			//bytesWritten, err = udpConn.WriteToUDP(buffer[totalBytesWritten:], resolved)
			bytesWritten, err = udpConn.Write(buffer[totalBytesWritten:])
			fmt.Println("byteWritten:", bytesWritten)
			fmt.Println("byteWritten conn addr:", udpConn.LocalAddr())
			if err != nil {
				fmt.Println("err byteWritten msg:", err.Error())
			}
		}

		if err != nil {
			log.Errorf("stream: failed to write entire buffer, err: %+v\n", err)
		}
		totalBytesWritten += bytesWritten
	}
	fmt.Println("*******************writerMutex.Unlock")
	writerMutex.Unlock()

	if err != nil {
		return errors.Wrap(err, "stream: failed to write to socket")
	}

	return nil
}

// receiveMessage reads, unmarshals and verifies a message from a net.Conn.
func (n *Network) receiveMessage(conn interface{}) (*protobuf.Message, error) {
	var err error

	// Read until all header bytes have been read.
	buffer := make([]byte, 4)

	bytesRead, totalBytesRead := 0, 0

	for totalBytesRead < 4 && err == nil {
		switch conn.(type) {
		case *net.TCPConn:
			bytesRead, err = conn.(*net.TCPConn).Read(buffer[totalBytesRead:])
		case *net.UDPConn:
			udpConn, _ := conn.(*net.UDPConn)
			_buffer := make([]byte, 4096)
			fmt.Printf("receiveMessage.conn.ptr:%v", conn)
			//bytesRead, _, err = udpConn.ReadFromUDP(_buffer[:])
			bytesRead, err = udpConn.Read(_buffer[:])
			fmt.Println("Read bytes from udpConn with ReadFrom method, bytesRead:", bytesRead, "buffer:", _buffer[:bytesRead])
			if err != nil {
				fmt.Println("err-msg:", err.Error())
			}
		default:
			return nil, errors.New("net connection type is ambiguous. default case is not allow.")
		}
		totalBytesRead += bytesRead
	}

	// Decode message size.
	size := binary.BigEndian.Uint32(buffer)

	if size == 0 {
		return nil, errEmptyMsg
	}

	if size > uint32(n.opts.recvBufferSize) {
		return nil, errors.Errorf("message has length of %d which is either broken or too large(default %d)", size, n.opts.recvBufferSize)
	}

	// Read until all message bytes have been read.
	buffer = make([]byte, size)

	bytesRead, totalBytesRead = 0, 0

	for totalBytesRead < int(size) && err == nil {
		switch conn.(type) {
		case *net.TCPConn:
			bytesRead, err = conn.(*net.TCPConn).Read(buffer[totalBytesRead:])
		case *net.UDPConn:
			udpConn, _ := conn.(*net.UDPConn)
			//bytesRead, _, err = udpConn.ReadFromUDP(buffer[totalBytesRead:])
			bytesRead, err = udpConn.Read(buffer[totalBytesRead:])
		default:
			return nil, errors.New("net connection type is ambiguous. default case is not allow.")
		}
		//bytesRead, err = conn.Read(buffer[totalBytesRead:])
		totalBytesRead += bytesRead
	}

	// Deserialize message.
	msg := new(protobuf.Message)
	fmt.Println("Receive buffer Message(byte):", buffer)
	err = proto.Unmarshal(buffer, msg)
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
