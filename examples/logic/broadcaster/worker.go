// Package broadcaster is a worker logic example, this will broadcast to all clients sending one message at a time
package broadcaster

import (
	"context"

	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/types"
)

type broadcasterWorker struct {
	GoBroke.LogicBase
	receive chan types.Message
}

func CreateWorker(broke *GoBroke.Broke, ctx context.Context) types.Logic {
	worker := broadcasterWorker{
		LogicBase: GoBroke.NewLogicBase("broadcaster", types.WORKER, broke),
		receive:   make(chan types.Message),
	}
	worker.startWorker()
	return &worker
}

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

func (w *broadcasterWorker) work(msg types.Message) {
	go func() {
		allClients := w.GetAllClients()
		for _, client := range allClients {
			sMsg := types.Message{
				ToClient:   []*clients.Client{client},
				FromLogic:  w,
				MessageRaw: msg.MessageRaw,
			}
			w.SendMessage(sMsg)
		}
	}()
}

func (w *broadcasterWorker) RunLogic(message types.Message) error {
	w.receive <- message
	return nil
}
