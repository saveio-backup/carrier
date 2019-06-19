package main

import (
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
	"fmt"
	"bufio"
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
	enableProxy := flag.Bool("enableProxy", false, "enable proxy")
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
	if protocol == "udp" && *enableProxy == true{
		builder.AddComponent(new(proxy.UDPProxyComponent))
	}
	if protocol == "kcp" && *enableProxy == true{
		builder.AddComponent(new(proxy.KCPProxyComponent))
	}
	if protocol == "quic" && *enableProxy== true{
		builder.AddComponent(new(proxy.QuicProxyComponent))
	}

	networkBuilder, err := builder.Build()
	if err != nil {
		log.Fatal(err)
		return
	}
	networkBuilder.SetProxyServer(proxyServer)
	go networkBuilder.Listen()
	networkBuilder.BlockUntilListening()
	if protocol == "udp" && *enableProxy == true{
		networkBuilder.BlockUntilUDPProxyFinish()
	}
	if protocol == "kcp" && *enableProxy == true{
		networkBuilder.BlockUntilKCPProxyFinish()
	}
	if protocol == "quic" && *enableProxy == true{
		networkBuilder.BlockUntilQuicProxyFinish()
	}

	if len(peers) > 0 {
		networkBuilder.Bootstrap(peers...)
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		input, _ := reader.ReadString('\n')
		fmt.Println("input:",input)
		log.Info("We Chat> ")
		// skip blank lines
		if len(strings.TrimSpace(input)) == 0 {
			continue
		}

		log.Infof("<%s> %s", networkBuilder.Address, input)

		//time.Sleep(time.Second*1)
		ctx := network.WithSignMessage(context.Background(), true)
		//fData, _:= readAllIntoMemory("./data")
		networkBuilder.Broadcast(ctx, &messages.ChatMessage{Message: fmt.Sprintf("%s",input)})
	}

}

func readAllIntoMemory(filename string) (content []byte, err error) {
	fp, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer fp.Close()

	fileInfo, err := fp.Stat()
	if err != nil {
		return nil, err
	}
	buffer := make([]byte, fileInfo.Size())
	fmt.Println("file.size:",fileInfo.Size())
	_, err = fp.Read(buffer)
	if err != nil {
		return nil, err
	}
	return buffer, nil
}
