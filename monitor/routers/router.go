package routers

import (
	"github.com/astaxie/beego"
	"github.com/saveio/carrier/monitor/controllers"
)

func init() {
	beego.Router("/", &controllers.MainController{})
}
