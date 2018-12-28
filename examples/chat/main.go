package main

import (
	"bufio"
	"context"
	"flag"
	"os"
	"strings"

	"github.com/golang/glog"
	"github.com/oniio/onip2p/crypto/ed25519"
	"github.com/oniio/onip2p/examples/chat/messages"
	"github.com/oniio/onip2p/network"
	"github.com/oniio/onip2p/network/discovery"
	"github.com/oniio/onip2p/types/opcode"
)

type ChatComponent struct{ *network.Component }

func (state *ChatComponent) Receive(ctx *network.ComponentContext) error {
	switch msg := ctx.Message().(type) {
	case *messages.ChatMessage:
		glog.Infof("<%s> %s", ctx.Client().ID.Address, msg.Message)
	}

	return nil
}

func main() {
	// glog defaults to logging to a file, override this flag to log to console for testing
	flag.Set("logtostderr", "true")

	// process other flags
	portFlag := flag.Int("port", 3000, "port to listen to")
	hostFlag := flag.String("host", "localhost", "host to listen to")
	protocolFlag := flag.String("protocol", "tcp", "protocol to use (kcp/tcp)")
	peersFlag := flag.String("peers", "", "peers to connect to")
	flag.Parse()

	port := uint16(*portFlag)
	host := *hostFlag
	protocol := *protocolFlag
	peers := strings.Split(*peersFlag, ",")

	keys := ed25519.RandomKeyPair()

	glog.Infof("Private Key: %s", keys.PrivateKeyHex())
	glog.Infof("Public Key: %s", keys.PublicKeyHex())

	opcode.RegisterMessageType(opcode.Opcode(1000), &messages.ChatMessage{})
	builder := network.NewBuilder()
	builder.SetKeys(keys)
	builder.SetAddress(network.FormatAddress(protocol, host, port))

	// Register peer discovery Component.
	builder.AddComponent(new(discovery.Component))

	// Add custom chat Component.
	builder.AddComponent(new(ChatComponent))

	net, err := builder.Build()
	if err != nil {
		glog.Fatal(err)
		return
	}

	go net.Listen()

	if len(peers) > 0 {
		net.Bootstrap(peers...)
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		input, _ := reader.ReadString('\n')

		// skip blank lines
		if len(strings.TrimSpace(input)) == 0 {
			continue
		}

		glog.Infof("<%s> %s", net.Address, input)

		ctx := network.WithSignMessage(context.Background(), true)
		net.Broadcast(ctx, &messages.ChatMessage{Message: input})
	}

}
