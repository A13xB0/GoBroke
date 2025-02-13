package inactivitymonitor

import (
	"fmt"
	"time"

	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/types"
)

type inactivityMonitor struct {
	GoBroke.LogicBase
	inactivityMinutes int
}

func CreateWorker(broke *GoBroke.Broke, inactivityMinutes int) types.Logic {
	worker := inactivityMonitor{
		LogicBase:         GoBroke.NewLogicBase("inactivitymonitor", types.PASSIVE, broke),
		inactivityMinutes: inactivityMinutes,
	}
	worker.startWorker()
	return &worker
}

func (w *inactivityMonitor) startWorker() {
	for {
		select {
		case <-w.Ctx.Done():
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
	return fmt.Errorf("this logic does not support invocation")
}
