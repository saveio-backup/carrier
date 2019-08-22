/**
 * Description:
 * Author: Yihen.Liu
 * Create: 2019-08-21
 */
package controllers

import (
	"os"

	"github.com/astaxie/beego"
	"github.com/saveio/carrier/network"
)

var Network *network.Network

func InitMonitor(network *network.Network) {
	beego.BConfig.WebConfig.ViewsPath = os.Getenv("GOPATH") + "/src/github.com/saveio/carrier/monitor/views"
	Network = network
}
