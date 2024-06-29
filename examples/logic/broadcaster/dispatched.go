package broadcaster

import (
	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/logic"
	"github.com/A13xB0/GoBroke/message"
)

type broadcasterDispatched struct {
	name  string
	lType logic.LogicType
	*GoBroke.Broke
}

func CreateDispatched(broke *GoBroke.Broke) logic.Logic {
	worker := broadcasterDispatched{
		name:  "broadcaster",
		lType: logic.DISPATCHED,
		Broke: broke,
	}
	return &worker
}

func (w *broadcasterDispatched) RunLogic(msg message.Message) error {
	clients := w.GetAllClients()
	sMsg := message.NewLogicMessage(w, clients, nil, msg.MessageRaw)
	w.SendMessage(sMsg)
	return nil
}

func (w *broadcasterDispatched) Type() logic.LogicType {
	return w.lType
}

func (w *broadcasterDispatched) Name() string {
	return w.name
}
