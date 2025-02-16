// Package inactivitymonitor provides a passive logic handler that monitors client activity
// and automatically removes clients that have been inactive for a specified duration.
package inactivitymonitor

import (
	"fmt"
	"time"

	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/types"
)

// This ensures this logic can be addressed from another logic
const Name types.LogicName = "broadcaster"

// inactivityMonitor implements a passive logic handler that periodically checks
// for and removes inactive clients from the broker.
type inactivityMonitor struct {
	GoBroke.LogicBase     // Embeds base logic functionality
	inactivityMinutes int // Duration in minutes after which a client is considered inactive
}

// CreateWorker creates a new inactivity monitor instance.
// Parameters:
//   - broke: The broker instance to monitor
//   - inactivityMinutes: The duration in minutes after which a client is considered inactive
//
// Returns a types.Logic interface that runs passively in the background.
func CreateWorker(broke *GoBroke.Broke, inactivityMinutes int) types.Logic {
	worker := inactivityMonitor{
		LogicBase:         GoBroke.NewLogicBase(Name, types.PASSIVE, broke),
		inactivityMinutes: inactivityMinutes,
	}
	worker.startWorker()
	return &worker
}

// startWorker begins the monitoring loop that checks for inactive clients.
// It runs continuously until the context is cancelled, checking client activity
// every 10 seconds and removing clients that exceed the inactivity threshold.
func (w *inactivityMonitor) startWorker() {
	for {
		select {
		case <-w.Ctx.Done():
		default:
			time.Sleep(10 * time.Second)
			clients := w.GetAllClients()
			for _, client := range clients {
				delta := time.Since(client.GetLastMessage())
				if delta.Minutes() > 15 {
					_ = w.RemoveClient(client)
				}
			}
		}
	}
}

// RunLogic implements the types.Logic interface.
// Since this is a passive monitor, it does not process messages and returns an error
// if invoked directly. All monitoring is handled by the background worker.
func (w *inactivityMonitor) RunLogic(message types.Message) error {
	return fmt.Errorf("this logic does not support invocation")
}
