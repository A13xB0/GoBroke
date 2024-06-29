// Package broadcaster is a worker logic example, this will broadcast to all clients sending one message at a time
package broadcaster

import (
	"context"
	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/logic"
	"github.com/A13xB0/GoBroke/message"
)

type broadcasterWorker struct {
	name    string
	lType   logic.LogicType
	receive chan message.Message
	ctx     context.Context
	*GoBroke.Broke
}

func CreateWorker(broke *GoBroke.Broke, ctx context.Context) logic.Logic {
	worker := broadcasterWorker{
		name:    "broadcaster",
		lType:   logic.WORKER,
		receive: make(chan message.Message),
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

func (w *broadcasterWorker) work(msg message.Message) {
	clients := w.GetAllClients()
	for _, client := range clients {
		sMsg := message.NewSimpleLogicMessage(w, client, nil, msg.MessageRaw)
		w.SendMessage(sMsg)
	}

}

func (w *broadcasterWorker) RunLogic(message message.Message) error {
	w.receive <- message
	return nil
}

func (w *broadcasterWorker) Type() logic.LogicType {
	return w.lType
}

func (w *broadcasterWorker) Name() string {
	return w.name
}
