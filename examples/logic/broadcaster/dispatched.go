// Package broadcaster provides example implementations of logic handlers for broadcasting messages.
package broadcaster

import (
	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/types"
)

// broadcasterDispatched implements a dispatched-type logic handler that broadcasts messages
// to all connected clients concurrently. Each message broadcast runs in its own goroutine.
type broadcasterDispatched struct {
	GoBroke.LogicBase // Embeds base logic functionality
}

// CreateDispatched creates a new broadcaster dispatched instance.
// It initializes the handler with the provided broker and returns it as a types.Logic interface.
// This implementation processes each message in a separate goroutine.
func CreateDispatched(broke *GoBroke.Broke) types.Logic {
	worker := broadcasterDispatched{
		LogicBase: GoBroke.NewLogicBase("broadcaster", types.DISPATCHED, broke),
	}

	return &worker
}

// RunLogic implements the types.Logic interface.
// It immediately broadcasts the received message to all connected clients.
// Since this is a dispatched handler, this method is called in its own goroutine
// for each message, allowing concurrent message broadcasting.
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
