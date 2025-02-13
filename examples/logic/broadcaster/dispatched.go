package broadcaster

import (
	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/types"
)

type broadcasterDispatched struct {
	name  string
	lType types.LogicType
	*GoBroke.Broke
}

func CreateDispatched(broke *GoBroke.Broke) types.Logic {
	worker := broadcasterDispatched{
		name:  "broadcaster",
		lType: types.DISPATCHED,
		Broke: broke,
	}
	return &worker
}

func (w *broadcasterDispatched) RunLogic(msg types.Message) error {
	clients := w.GetAllClients()
	sMsg := types.Message{
		ToClient:   clients,
		FromLogic:  w,
		MessageRaw: msg.MessageRaw,
	}
	w.SendMessage(sMsg)
	return nil
}

func (w *broadcasterDispatched) Type() types.LogicType {
	return w.lType
}

func (w *broadcasterDispatched) Name() string {
	return w.name
}
