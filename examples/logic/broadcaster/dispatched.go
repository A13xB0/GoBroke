// Package broadcaster provides example implementations of logic handlers for broadcasting messages.
// It shows the same job as a DISPATCHED logic and as a WORKER logic.
package broadcaster

import (
	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/types"
)

// DispatchedName is the logic name of the DISPATCHED broadcaster.
const DispatchedName types.LogicName = "broadcaster.dispatched"

// broadcasterDispatched broadcasts each message on its own goroutine, so
// several broadcasts may run at once and finish in any order.
type broadcasterDispatched struct {
	GoBroke.LogicBase
}

// CreateDispatched creates a DISPATCHED broadcaster.
func CreateDispatched(broke *GoBroke.Broke) types.Logic {
	return &broadcasterDispatched{
		LogicBase: GoBroke.NewLogicBase(DispatchedName, types.DISPATCHED, broke),
	}
}

// RunLogic sends the message to every connected client.
func (w *broadcasterDispatched) RunLogic(msg types.Message) error {
	w.SendMessageQuickly(types.Message{
		ToClient:   w.GetAllClients(),
		FromLogic:  w.Name(),
		MessageRaw: msg.MessageRaw,
	})
	return nil
}
