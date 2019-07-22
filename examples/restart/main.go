/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-07-19 
*/
package main

import (
	"github.com/saveio/edge/p2p/network"
	"github.com/saveio/carrier/crypto"
	"time"
	"fmt"
)

func main() {
	networkKey := &crypto.KeyPair{
		PublicKey:  []byte("dspPub"),
		PrivateKey: []byte("dspPrivate"),
	}
	Network:=network.NewP2P()
	Network.SetNetworkKey(networkKey)
	Network.SetProxyServer("tcp://192.168.1.115:6008")
	Network.Start("tcp://192.168.1.115:3004")
	time.Sleep(time.Second*3)
	fmt.Println("begin to stop, sleep 10s...")
	Network.Stop()
	time.Sleep(time.Second*10)
	fmt.Println("begin to restart...")
	Network.Start("tcp://192.168.1.115:3004")
	select {}
}