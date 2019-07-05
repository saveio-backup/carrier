package main

import (
	"os"
	"os/signal"
	"syscall"
	"github.com/saveio/themis/common/log"
	"github.com/saveio/carrier/examples/channel_test/transport"
)

const PROTOCOL = "quic"

func main() {
	trans := transport.NewTransport()
	trans.Network.Start(PROTOCOL + "://127.0.0.1:3112")
	waitToExit()
}

func waitToExit() {
	exit := make(chan bool, 0)
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		for sig := range sc {
			log.Infof("server received exit signal:%v.", sig.String())
			close(exit)
			break
		}
	}()
	<-exit
}