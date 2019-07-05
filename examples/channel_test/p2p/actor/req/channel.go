package req

import (
	"github.com/ontio/ontology-eventbus/actor"
)

var ChannelServerPid *actor.PID

func SetChannelPid(conPid *actor.PID) {
	ChannelServerPid = conPid
}
