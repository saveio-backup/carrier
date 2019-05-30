package main

import (
	"bufio"
	"context"
	"flag"
	"github.com/saveio/carrier/crypto/ed25519"
	"github.com/saveio/carrier/examples/chat/messages"
	"github.com/saveio/carrier/network"
	"github.com/saveio/carrier/network/components/discovery"
	"github.com/saveio/carrier/network/components/keepalive"
	"github.com/saveio/carrier/network/components/proxy"
	"github.com/saveio/carrier/types/opcode"
	"github.com/saveio/themis/common/log"
	"os"
	"strings"
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
	hostFlag := flag.String("host", "127.0.0.1", "local host to listen to")
	protocolFlag := flag.String("protocol", "udp", "protocol to use (kcp/tcp/udp)")
	peersFlag := flag.String("peers", "", "peers to connect to")
	proxyFlag := flag.String("proxy", "localhost", "proxy server ip")
	flag.Parse()

	port := uint16(*portFlag)
	host := *hostFlag
	protocol := *protocolFlag
	peers := strings.Split(*peersFlag, ",")
	proxyServer := *proxyFlag
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

	// Register peer discovery Component.
	builder.AddComponent(new(discovery.Component))

	// Add custom chat Component.
	builder.AddComponent(new(ChatComponent))
	if protocol == "udp" {
		builder.AddComponent(new(proxy.UDPProxyComponent))
	}
	if protocol == "kcp" {
		builder.AddComponent(new(proxy.KCPProxyComponent))
	}

	networkBuilder, err := builder.Build()
	if err != nil {
		log.Fatal(err)
		return
	}
	networkBuilder.SetProxyServer(proxyServer)
	go networkBuilder.Listen()
	networkBuilder.BlockUntilListening()
	if protocol == "udp" {
		networkBuilder.BlockUntilUDPProxyFinish()
	}
	if protocol == "kcp" {
		networkBuilder.BlockUntilKCPProxyFinish()
	}

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
