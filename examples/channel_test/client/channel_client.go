package main

import (
	"time"

	"github.com/saveio/carrier/examples/channel_test/transport"
	"github.com/saveio/carrier/examples/channel_test/common/messages"
	"github.com/saveio/themis/common/log"
)

const PROTOCOL = "quic"

func main() {
	trans := transport.NewTransport()

	err := trans.Network.Start(PROTOCOL + "://127.0.0.1:3111")
	if err != nil {
		log.Errorf("[Start]: ", err.Error())
	}

	err = trans.Network.Connect(PROTOCOL + "://127.0.0.1:3112")
	if err != nil {
		log.Errorf("[Connect]: ", err.Error())
	}

	for id := uint64(0); ; id++ {
		message := &messages.Message{
			Message: "hello",
			MessageId: &messages.MessageID{
				MessageId: id,
			},
		}
		trans.SendAsync(PROTOCOL + "://127.0.0.1:3112", message)
		time.Sleep(10* time.Millisecond)
	}
}