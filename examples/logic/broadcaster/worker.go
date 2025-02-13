// Package broadcaster is a worker logic example, this will broadcast to all clients sending one message at a time
package broadcaster

import (
	"context"

	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/types"
)

type broadcasterWorker struct {
	name    string
	lType   types.LogicType
	receive chan types.Message
	ctx     context.Context
	*GoBroke.Broke
}

func CreateWorker(broke *GoBroke.Broke, ctx context.Context) types.Logic {
	worker := broadcasterWorker{
		name:    "broadcaster",
		lType:   types.WORKER,
		receive: make(chan types.Message),
		Broke:   broke,
		ctx:     ctx,
	}
	worker.startWorker()
	return &worker
}

func (w *broadcasterWorker) startWorker() {
	for {
		select {
		case <-w.ctx.Done():
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

func (w *broadcasterWorker) Type() types.LogicType {
	return w.lType
}

func (w *broadcasterWorker) Name() string {
	return w.name
}
