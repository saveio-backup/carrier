package main

import (
	"context"
	"flag"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/saveio/carrier/crypto/ed25519"
	"github.com/saveio/carrier/examples/request_benchmark/messages"
	"github.com/saveio/carrier/network"
	"github.com/saveio/carrier/network/components/discovery"
	"github.com/saveio/carrier/types/opcode"
	"github.com/saveio/themis/common/log"
)

const (
	defaultNumNodes      = 5
	defaultNumReqPerNode = 50
	host                 = "localhost"
	startPort            = 23000
)

func init() {
	opcode.RegisterMessageType(opcode.Opcode(1000), &messages.LoadRequest{})
	opcode.RegisterMessageType(opcode.Opcode(1001), &messages.LoadReply{})
}

func main() {
	// send glog to the terminal instead of a file
	flag.Set("logtostderr", "true")

	fmt.Print(run())
}

func run() string {

	runtime.GOMAXPROCS(runtime.NumCPU())

	numReqPerNodeFlag := flag.Int("r", defaultNumReqPerNode, "Number of requests per node")
	numNodesFlag := flag.Int("n", defaultNumNodes, "Number of nodes")

	flag.Parse()

	numNodes := *numNodesFlag
	numReqPerNode := *numReqPerNodeFlag

	nets := setupNetworks(host, startPort, numNodes)
	expectedTotalResp := numReqPerNode * numNodes * (numNodes - 1)
	var totalPos uint32

	startTime := time.Now()

	wg := &sync.WaitGroup{}

	// sending to all nodes concurrently
	for r := 0; r < numReqPerNode; r++ {
		for n, nt := range nets {
			wg.Add(1)
			go func(net *network.Network, idx int) {
				defer wg.Done()
				positive := sendMsg(net, idx)
				atomic.AddUint32(&totalPos, positive)
			}(nt, n+numNodes*r)
		}
	}
	wg.Wait()

	totalTime := time.Since(startTime)
	reqPerSec := float64(totalPos) / totalTime.Seconds()

	return fmt.Sprintf("Test completed in %s, num nodes = %d, successful requests = %d / %d, requestsPerSec = %f\n",
		totalTime, numNodes, totalPos, expectedTotalResp, reqPerSec)
}

func setupNetworks(host string, startPort int, numNodes int) []*network.Network {
	var nodes []*network.Network

	for i := 0; i < numNodes; i++ {
		builder := network.NewBuilder()
		builder.SetKeys(ed25519.RandomKeyPair())
		builder.SetAddress(network.FormatAddress("tcp", host, uint16(startPort+i)))

		builder.AddComponent(new(discovery.Component))
		builder.AddComponent(new(loadTestComponent))

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
			node.Bootstrap([]string{nodes[0].Address}, []string{"client-id"})
		}
	}

	// Wait for all nodes to finish discovering other peers.
	time.Sleep(1 * time.Second)

	return nodes
}

func sendMsg(net *network.Network, idx int) uint32 {
	var positiveResponses uint32

	Component, registered := net.Component(discovery.ComponentID)
	if !registered {
		return 0
	}

	routes := Component.(*discovery.Component).Routes

	addresses := routes.GetPeerAddresses()

	errs := make(chan error, len(addresses))

	wg := &sync.WaitGroup{}

	for _, address := range addresses {
		wg.Add(1)

		go func(address string) {
			defer wg.Done()

			expectedID := fmt.Sprintf("%s:%d->%s", net.Address, idx, address)

			client, err := net.Client(address, "client-id")
			if err != nil {
				errs <- errors.Wrapf(err, "client error for req id %s", expectedID)
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			response, err := client.Request(ctx, &messages.LoadRequest{Id: expectedID}, 1*time.Second)
			if err != nil {
				errs <- errors.Wrapf(err, "request error for req id %s", expectedID)
				return
			}

			if reply, ok := response.(*messages.LoadReply); ok {
				if reply.Id == expectedID {
					atomic.AddUint32(&positiveResponses, 1)
				} else {
					errs <- errors.Errorf("expected ID=%s got %s\n", expectedID, reply.Id)
				}
			} else {
				errs <- errors.Errorf("expected messages.LoadReply but got %v\n", response)
			}

		}(address)
	}

	wg.Wait()

	close(errs)

	for err := range errs {
		log.Error(err)
	}

	return atomic.LoadUint32(&positiveResponses)
}

type loadTestComponent struct {
	*network.Component
}

// Receive takes in *messages.ProxyMessage and replies with *messages.ID
func (p *loadTestComponent) Receive(ctx *network.ComponentContext) error {
	switch msg := ctx.Message().(type) {
	case *messages.LoadRequest:
		response := &messages.LoadReply{Id: msg.Id}
		ctx.Reply(context.Background(), response)
	}

	return nil
}
