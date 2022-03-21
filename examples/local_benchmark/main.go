/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-06-14
 */
package main

import (
	"flag"

	"github.com/saveio/carrier/examples/local_benchmark/receiver"
	"github.com/saveio/carrier/examples/local_benchmark/sender"
)

func main() {
	protocol := flag.String("protocol", "kcp", "protocol to use (kcp/tcp/udp)")
	flag.Parse()
	go receiver.Run(*protocol)
	go sender.Run(*protocol)
	select {}
}
