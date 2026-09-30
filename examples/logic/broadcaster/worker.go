package broadcaster

import (
	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/types"
)

// WorkerName is the logic name of the WORKER broadcaster.
const WorkerName types.LogicName = "broadcaster.worker"

// broadcasterWorker broadcasts one message at a time, in the order they
// arrive. The broker gives a WORKER its own lane, so it never needs to start
// goroutines of its own and never delays other logics.
type broadcasterWorker struct {
	GoBroke.LogicBase
}

// CreateWorker creates a WORKER broadcaster.
func CreateWorker(broke *GoBroke.Broke) types.Logic {
	return &broadcasterWorker{
		LogicBase: GoBroke.NewLogicBase(WorkerName, types.WORKER, broke),
	}
}

// RunLogic sends the message to every connected client.
func (w *broadcasterWorker) RunLogic(msg types.Message) error {
	w.SendMessageQuickly(types.Message{
		ToClient:   w.GetAllClients(),
		FromLogic:  w.Name(),
		MessageRaw: msg.MessageRaw,
	})
	return nil
}
