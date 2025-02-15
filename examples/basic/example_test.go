package basic

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/saveio/carrier/crypto/ed25519"
	"github.com/saveio/carrier/examples/basic/messages"
	"github.com/saveio/carrier/network"
	"github.com/saveio/carrier/network/components/discovery"
	"github.com/saveio/carrier/types/opcode"
)

// BasicComponent buffers all messages into a mailbox for this test.
type BasicComponent struct {
	*network.Component
	Mailbox chan *messages.BasicMessage
}

func (state *BasicComponent) Startup(net *network.Network) {
	// Create mailbox
	state.Mailbox = make(chan *messages.BasicMessage, 1)
}

func (state *BasicComponent) Receive(ctx *network.ComponentContext) error {
	switch msg := ctx.Message().(type) {
	case *messages.BasicMessage:
		state.Mailbox <- msg
	}
	return nil
}

// ExampleBasicComponent demonstrates how to broadcast a message to a set of peers that discover
// each other through peer discovery.
func ExampleBasicComponent() {
	flag.Parse()

	numNodes := 3

	host := "localhost"
	startPort := 5000

	var nodes []*network.Network
	var Components []*BasicComponent

	for i := 0; i < numNodes; i++ {
		builder := network.NewBuilder()
		builder.SetKeys(ed25519.RandomKeyPair())
		builder.SetAddress(network.FormatAddress("tcp", host, uint16(startPort+i)))

		builder.AddComponent(new(discovery.Component)) //dht

		Components = append(Components, new(BasicComponent))
		builder.AddComponent(Components[i])

		node, err := builder.Build()
		if err != nil {
			fmt.Println(err)
		}

		go node.Listen()

		nodes = append(nodes, node)
	}

	// Make sure all nodes are listening for incoming peers.
	for _, node := range nodes {
		node.BlockUntilListening()
	}

	// Bootstrap to Node 0.
	for i, node := range nodes {
		if i != 0 {
			node.Bootstrap(nodes[0].Address)
		}
	}

	opcode.RegisterMessageType(opcode.Opcode(1000), &messages.BasicMessage{})

	// Wait for all nodes to finish discovering other peers.
	time.Sleep(1 * time.Second)

	// Broadcast out a message from Node 0.
	expected := "This is a broadcasted message from Node 0."
	nodes[0].Broadcast(context.Background(), &messages.BasicMessage{Message: expected})

	fmt.Println("Node 0 sent out a message.")

	// Check if message was received by other nodes.
	for i := 1; i < len(nodes); i++ {
		select {
		case received := <-Components[i].Mailbox:
			if received.Message != expected {
				fmt.Printf("Expected message %s to be received by node %d but got %v\n", expected, i, received.Message)
			} else {
				fmt.Printf("Node %d received a message from Node 0.\n", i)
			}
		case <-time.After(3 * time.Second):
			fmt.Printf("Timed out attempting to receive message from Node 0.\n")
		}
	}

	// Output:
	// Node 0 sent out a message.
	// Node 1 received a message from Node 0.
	// Node 2 received a message from Node 0.
}
