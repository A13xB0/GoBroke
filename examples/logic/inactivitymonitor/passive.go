package inactivitymonitor

import (
	"context"
	"fmt"
	"time"

	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/types"
)

type inactivityMonitor struct {
	name              string
	lType             types.LogicType
	inactivityMinutes int
	ctx               context.Context
	*GoBroke.Broke
}

func CreateWorker(broke *GoBroke.Broke, inactivityMinutes int, ctx context.Context) types.Logic {
	worker := inactivityMonitor{
		name:              "inactivitymonitor",
		lType:             types.PASSIVE,
		Broke:             broke,
		inactivityMinutes: inactivityMinutes,
		ctx:               ctx,
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

func (w *inactivityMonitor) RunLogic(message types.Message) error {
	return fmt.Errorf("This logic does not support invocation")
}

func (w *inactivityMonitor) Type() types.LogicType {
	return w.lType
}

func (w *inactivityMonitor) Name() string {
	return w.name
}
