package main

import (
	"bufio"
	"context"
	"flag"
	"os"
	"strings"
	"net"
	"github.com/oniio/oniChain/common/log"
	"github.com/oniio/oniP2p/crypto/ed25519"
	"github.com/oniio/oniP2p/examples/chat/messages"
	"github.com/oniio/oniP2p/network"
	"github.com/oniio/oniP2p/network/discovery"
	"github.com/oniio/oniP2p/network/keepalive"
	"github.com/oniio/oniP2p/network/nat"
	"github.com/oniio/oniP2p/types/opcode"
)

const (
	udp = "udp4"
)
type ChatComponent struct{ *network.Component }

func (state *ChatComponent) Receive(ctx *network.ComponentContext) error {
	switch msg := ctx.Message().(type) {
	case *messages.ChatMessage:
		log.Infof("<%s> %s", ctx.Client().ID.Address, msg.Message)
	}

	return nil
}

func main() {
	flag.Set("logtostderr", "true")

	// process other flags
	portFlag := flag.Int("port", 60002, "local port to listen to")
	hostFlag := flag.String("host", nat.GetValidLocalIP().String(), "local host to listen to")
	protocolFlag := flag.String("protocol", "udp", "protocol to use (kcp/tcp/udp)")
	peersFlag := flag.String("peers", "", "peers to connect to")
	natFlag := flag.Bool("stun", true, "enable nat traversal")
	flag.Parse()

	port := uint16(*portFlag)
	host := *hostFlag
	protocol := *protocolFlag
	peers := strings.Split(*peersFlag, ",")
	natEnabled := *natFlag
	keys := ed25519.RandomKeyPair()

	log.Infof("Private Key: %s", keys.PrivateKeyHex())
	log.Infof("Public Key: %s", keys.PublicKeyHex())

	opcode.RegisterMessageType(opcode.Opcode(1000), &messages.ChatMessage{})
	builder := network.NewBuilder()
	builder.SetKeys(keys)
	builder.SetAddress(network.FormatAddress(protocol, host, port))

	// Add keepalive Component
	peerStateChan := make(chan *keepalive.PeerStateEvent, 10)
	options := []keepalive.ComponentOption{
		keepalive.WithKeepaliveInterval(keepalive.DefaultKeepaliveInterval),
		keepalive.WithKeepaliveTimeout(keepalive.DefaultKeepaliveTimeout),
		keepalive.WithPeerStateChan(peerStateChan),
	}
	builder.AddComponent(keepalive.New(options...))

	// Register NAT traversal Component.
	if natEnabled {
		nat.RegisterStunComponent(builder)
	}
	// Register peer discovery Component.
	builder.AddComponent(new(discovery.Component))

	// Add custom chat Component.
	builder.AddComponent(new(ChatComponent))

	networkBuilder, err := builder.Build()
	if err != nil {
		log.Fatal(err)
		return
	}
	lAddr, err := net.ResolveUDPAddr(udp, host)
	conn, err := net.ListenUDP(udp, lAddr)
	networkBuilder.Conn=conn
	go networkBuilder.Listen()
	if len(peers) > 0 {
		networkBuilder.Bootstrap(peers...)
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		input, _ := reader.ReadString('\n')
		log.Info("We Chat> ")
		// skip blank lines
		if len(strings.TrimSpace(input)) == 0 {
			continue
		}

		log.Infof("<%s> %s", networkBuilder.Address, input)

		ctx := network.WithSignMessage(context.Background(), true)
		networkBuilder.Broadcast(ctx, &messages.ChatMessage{Message: input})
	}

}
