package inactivitymonitor

import (
	"context"
	"fmt"
	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/logic"
	"github.com/A13xB0/GoBroke/message"
	"time"
)

type inactivityMonitor struct {
	name              string
	lType             logic.LogicType
	inactivityMinutes int
	ctx               context.Context
	*GoBroke.Broke
}

func CreateWorker(broke *GoBroke.Broke, inactivityMinutes int, ctx context.Context) logic.Logic {
	worker := inactivityMonitor{
		name:              "inactivitymonitor",
		lType:             logic.PASSIVE,
		Broke:             broke,
		inactivityMinutes: inactivityMinutes,
	}
	worker.startWorker()
	return &worker
}

func (w *inactivityMonitor) startWorker() {
	for {
		select {
		case <-w.ctx.Done():
		default:
			time.Sleep(10 * time.Second)
			clients := w.GetAllClients()
			for _, client := range clients {
				delta := time.Now().Sub(client.GetLastMessage())
				if delta.Minutes() > 15 {
					_ = w.RemoveClient(client)
				}
			}
		}
	}
}

func (w *inactivityMonitor) RunLogic(message message.Message) error {
	return fmt.Errorf("This logic does not support invocation")
}

func (w *inactivityMonitor) Type() logic.LogicType {
	return w.lType
}

func (w *inactivityMonitor) Name() string {
	return w.name
}
