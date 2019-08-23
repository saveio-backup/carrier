package controllers

import (
	"reflect"

	"github.com/astaxie/beego"
	"github.com/saveio/carrier/network"
)

type MainController struct {
	beego.Controller
}
type Proxy struct {
	Enable          bool
	Servers         []string
	WorkingServer   string
	ConnectionState string
}

func getComponentList() []string {
	var components []string
	for _, v := range Network.Components.GetInstallComponents() {
		components = append(components, reflect.TypeOf(v.Component).String())
	}
	return components
}

func getProxyConnState() string {
	peerState, _ := Network.GetRealConnState(Network.GetWorkingProxyServer())
	if peerState == network.PEER_REACHABLE {
		return "PEER_REACHABLE"
	}
	if peerState == network.PEER_UNREACHABLE {
		return "PEER_UNREACHABLE"
	}
	if peerState == network.PEER_UNKNOWN {
		return "PEER_UNKNOWN"
	}
	return ""
}
func (c *MainController) Get() {
	c.Data["Network"] = Network
	c.Data["NetworkID"] = Network.GetNetworkID()
	c.Data["Version"] = network.Version
	c.Data["Proxy"] = &Proxy{
		Enable:          Network.ProxyModeEnable(),
		WorkingServer:   Network.GetWorkingProxyServer(),
		Servers:         Network.ProxyService.Servers,
		ConnectionState: getProxyConnState(),
	}
	c.Data["Components"] = getComponentList()
	c.Data["Clients"] = Network
	c.Data["Connections"] = Network
	c.TplName = "index.html"
}
