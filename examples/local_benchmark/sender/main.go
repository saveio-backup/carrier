package sender

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
	"strconv"
	"time"

	"context"

	"github.com/saveio/carrier/crypto/ed25519"
	"github.com/saveio/carrier/examples/local_benchmark/messages"
	"github.com/saveio/carrier/network"
	"github.com/saveio/carrier/types/opcode"
)

var profile = flag.String("profile", "", "write cpu profile to file")
var port = flag.Uint("port", 3002, "port to listen on")
var receiver = map[string]string{
	"udp":  "udp://127.0.0.1:3001",
	"tcp":  "tcp://127.0.0.1:3001",
	"kcp":  "kcp://127.0.0.1:3001",
	"quic": "quic://127.0.0.1:3001",
}

func Run(protocol string) {
	flag.Set("logtostderr", "true")

	go func() {
		log.Println(http.ListenAndServe("127.0.0.1:7070", nil))
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

	if *profile != "" {
		f, err := os.Create(*profile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
	}

	builder := network.NewBuilder()
	builder.SetAddress(protocol + "://127.0.0.1:" + strconv.Itoa(int(*port)))
	builder.SetKeys(ed25519.RandomKeyPair())

	net, err := builder.Build()
	if err != nil {
		panic(err)
	}

	go net.Listen()
	net.Bootstrap(receiver[protocol])

	time.Sleep(500 * time.Millisecond)

	fmt.Printf("Spamming messages to %s...\n", receiver[protocol])

	client, err := net.Client(receiver[protocol])
	if err != nil {
		panic(err)
	}

	ctx := network.WithSignMessage(context.Background(), true)
	count := 0
	go func() {
		for range time.Tick(1 * time.Second) {
			fmt.Printf("Send %d messages.\n", count)

			count = 0
		}

	}()
	for {
		err = client.Tell(ctx, &messages.BasicMessage{})
		count += 1
		if err != nil {
			panic(err)
		}
	}
}
