package monitor

import (
	"github.com/astaxie/beego"
	"github.com/saveio/carrier/monitor/controllers"
	_ "github.com/saveio/carrier/monitor/routers"
	"github.com/saveio/carrier/network"
)

func Run(network *network.Network) {
	controllers.InitMonitor(network)
	beego.Run()
}
