package opcode

import (
	"reflect"
	"sync"

	"github.com/saveio/carrier/internal/protobuf"

	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
)

func init() {
	msgOpcodePairs := []struct {
		msg    proto.Message
		opcode Opcode
	}{
		{&protobuf.Bytes{}, BytesCode},
		{&protobuf.Ping{}, PingCode},
		{&protobuf.Pong{}, PongCode},
		{&protobuf.LookupNodeRequest{}, LookupNodeRequestCode},
		{&protobuf.LookupNodeResponse{}, LookupNodeResponseCode},
		{&protobuf.Keepalive{}, KeepaliveCode},
		{&protobuf.KeepaliveResponse{}, KeepaliveResponseCode},
		{&protobuf.Disconnect{}, DisconnectCode},
		{&protobuf.ProxyRequest{}, ProxyRequestCode},
		{&protobuf.ProxyResponse{}, ProxyResponseCode},
		{&protobuf.MetricRequest{}, MetricRequestCode},
		{&protobuf.MetricResponse{}, MetricResponseCode},
		{&protobuf.AsyncAckResponse{}, AckResponseCode},
	}

	for _, pair := range msgOpcodePairs {
		opcodeTbl.Store(pair.opcode, pair.msg)
		t := reflect.TypeOf(pair.msg)
		msgTbl.Store(t, pair.opcode)
	}
}

type Opcode uint32

const (
	UnregisteredCode       Opcode = 0x00000 // 0
	BytesCode              Opcode = 0x00001 // 1
	KeepaliveCode          Opcode = 0x00002 // 2
	KeepaliveResponseCode  Opcode = 0x00003 // 3
	ProxyResponseCode      Opcode = 0x00009 // 9
	PingCode               Opcode = 0x0000a // 10
	PongCode               Opcode = 0x0000b // 11
	LookupNodeRequestCode  Opcode = 0x0000c // 12
	LookupNodeResponseCode Opcode = 0x0000d // 13
	DisconnectCode         Opcode = 0x0000e // 14
	ProxyRequestCode       Opcode = 0x0000f // 15
	MetricRequestCode      Opcode = 0x00010 // 16
	MetricResponseCode     Opcode = 0x00011 // 17
	AckResponseCode        Opcode = 0x00012 // 18

	ApplicationOpCodeStart Opcode = 1000
)

var (
	// opcodeTbl is a map of <Opcode, proto.Message> pairs
	opcodeTbl = sync.Map{}
	// msgTbl is a map of <reflect.Type, Opcode> pairs
	msgTbl = sync.Map{}
)

// RegisterMessageType registers a new proto message to the given opcode
func RegisterMessageType(opcode Opcode, msg proto.Message) error {
	// reserve first 1000 opcodes
	if opcode < ApplicationOpCodeStart {
		return errors.New("types: opcode must be 1000 or greater")
	}
	raw, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	if len(raw) != 0 {
		return errors.New("types: must provide an empty protobuf message")
	}
	if _, loaded := opcodeTbl.LoadOrStore(opcode, msg); loaded {
		return errors.New("types: opcode already exists, choose a different opcode")
	} else {
		msgTbl.Store(reflect.TypeOf(msg), opcode)
	}
	return nil
}

// GetMessageType returns the corresponding proto message type given an opcode
func GetMessageType(code Opcode) (proto.Message, error) {
	if i, ok := opcodeTbl.Load(code); ok {
		return proto.Clone(i.(proto.Message)), nil
	}
	return nil, errors.New("types: opcode not found, did you register it?")
}

// GetOpcode returns the corresponding opcode given a proto message
func GetOpcode(msg proto.Message) (Opcode, error) {
	t := reflect.TypeOf(msg)
	if i, ok := msgTbl.Load(t); ok {
		return i.(Opcode), nil
	}
	return UnregisteredCode, errors.New("types: message type not found, did you register it?")
}
