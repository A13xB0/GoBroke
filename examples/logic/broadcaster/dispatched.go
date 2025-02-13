package broadcaster

import (
	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/types"
)

type broadcasterDispatched struct {
	GoBroke.LogicBase
}

func CreateDispatched(broke *GoBroke.Broke) types.Logic {
	worker := broadcasterDispatched{
		LogicBase: GoBroke.NewLogicBase("broadcaster", types.DISPATCHED, broke),
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
