package receiver

import _ "net/http/pprof"

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/saveio/carrier/crypto/ed25519"
	"github.com/saveio/carrier/examples/local_benchmark/messages"
	"github.com/saveio/carrier/network"
	"github.com/saveio/carrier/types/opcode"
)

type BenchmarkComponent struct {
	*network.Component
	counter int
}

func (state *BenchmarkComponent) Receive(ctx *network.ComponentContext) error {
	switch ctx.Message().(type) {
	case *messages.BasicMessage:
		state.counter++
	}

	return nil
}

var _profile = flag.String("_profile", "", "write cpu _profile to file")
var Address = map[string]string{
	"tcp":  "tcp://127.0.0.1:3001",
	"udp":  "udp://127.0.0.1:3001",
	"kcp":  "kcp://127.0.0.1:3001",
	"quic": "quic://127.0.0.1:3001",
}

func Run(protocol string) {
	flag.Set("logtostderr", "true")

	go func() {
		log.Println(http.ListenAndServe("127.0.0.1:6060", nil))
	}()

	runtime.GOMAXPROCS(runtime.NumCPU())
	opcode.RegisterMessageType(opcode.Opcode(1000), &messages.BasicMessage{})

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		<-c
		pprof.StopCPUProfile()
		os.Exit(0)
	}()

	if *_profile != "" {
		f, err := os.Create(*_profile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
	}

	builder := network.NewBuilder()
	builder.SetAddress(Address[protocol])
	builder.SetKeys(ed25519.RandomKeyPair())

	state := new(BenchmarkComponent)
	builder.AddComponent(state)

	net, err := builder.Build()
	if err != nil {
		panic(err)
	}

	go net.Listen()

	fmt.Println("Waiting for sender on ", Address[protocol])

	// Run loop every 1 second.
	for range time.Tick(1 * time.Second) {
		fmt.Printf("Got %d messages.\n", state.counter)

		state.counter = 0
	}
}
