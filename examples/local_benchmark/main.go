/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-06-14
 */
package main

import (
	"flag"
	"os"

	"github.com/saveio/carrier/examples/local_benchmark/receiver"
	"github.com/saveio/carrier/examples/local_benchmark/sender"
)

func main() {
	protocol := flag.String("protocol", "kcp", "protocol to use (kcp/tcp/udp)")
	flag.Parse()
	arg := os.Args[1]
	if arg == "1" {
		go receiver.Run(*protocol)
	} else if arg=="2" {
		go sender.Run(*protocol)
	} else {
		go receiver.Run(*protocol)
		go sender.Run(*protocol)
	}
	select {}
}
