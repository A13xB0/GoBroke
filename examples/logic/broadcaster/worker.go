// Package broadcaster provides example implementations of logic handlers for broadcasting messages.
// This package demonstrates how to implement both worker and dispatched broadcasting patterns.
package broadcaster

import (
	"context"

	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/types"
)

// broadcasterWorker implements a worker-type logic handler that broadcasts messages
// to all connected clients sequentially, one message at a time.
type broadcasterWorker struct {
	GoBroke.LogicBase                    // Embeds base logic functionality
	receive           chan types.Message // Channel for receiving messages to broadcast
}

// CreateWorker creates a new broadcaster worker instance.
// It initializes the worker with the provided broker and context,
// starts the worker's processing loop, and returns it as a types.Logic interface.
func CreateWorker(broke *GoBroke.Broke, ctx context.Context) types.Logic {
	worker := broadcasterWorker{
		LogicBase: GoBroke.NewLogicBase("broadcaster", types.WORKER, broke),
		receive:   make(chan types.Message),
	}
	worker.startWorker()
	return &worker
}

// startWorker begins the worker's message processing loop.
// It continuously listens for messages on the receive channel
// and processes them until the context is cancelled.
func (w *broadcasterWorker) startWorker() {
	for {
		select {
		case <-w.Ctx.Done():
			return
		case msg := <-w.receive:
			w.work(msg)
		}
	}
}

// work processes a single message by broadcasting it to all connected clients.
// It runs in a separate goroutine to avoid blocking the main processing loop.
func (w *broadcasterWorker) work(msg types.Message) {
	go func() {
		allClients := w.GetAllClients()
		for _, client := range allClients {
			sMsg := types.Message{
				ToClient:   []*clients.Client{client},
				FromLogic:  w.Name(),
				MessageRaw: msg.MessageRaw,
			}
			w.SendMessage(sMsg)
		}
	}()
}

// RunLogic implements the types.Logic interface.
// It queues the received message for broadcasting and returns immediately.
// The actual broadcasting is handled asynchronously by the worker loop.
func (w *broadcasterWorker) RunLogic(message types.Message) error {
	w.receive <- message
	return nil
}
