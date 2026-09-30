// Package inactivitymonitor provides a passive logic handler that monitors client activity
// and automatically removes clients that have been inactive for a specified duration.
package inactivitymonitor

import (
	"context"
	"errors"
	"time"

	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/types"
)

// Name lets other logics refer to this one.
const Name types.LogicName = "inactivitymonitor"

// errNotInvocable is returned if a message is ever routed to this logic.
var errNotInvocable = errors.New("inactivitymonitor receives no messages")

// inactivityMonitor is a PASSIVE logic with a Run loop: it receives no
// messages, and the broker starts Run in Start and cancels it on shutdown.
type inactivityMonitor struct {
	GoBroke.LogicBase
	timeout time.Duration
	every   time.Duration
}

// Create returns a monitor that removes clients idle for longer than timeout.
func Create(broke *GoBroke.Broke, timeout time.Duration) types.Logic {
	return &inactivityMonitor{
		LogicBase: GoBroke.NewLogicBase(Name, types.PASSIVE, broke),
		timeout:   timeout,
		every:     10 * time.Second,
	}
}

// Run checks for idle clients until ctx is cancelled.
func (w *inactivityMonitor) Run(ctx context.Context) error {
	t := time.NewTicker(w.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			w.removeIdle()
		}
	}
}

func (w *inactivityMonitor) removeIdle() {
	for _, c := range w.GetAllClients() {
		if time.Since(c.GetLastMessage()) > w.timeout {
			_ = w.RemoveClient(c)
		}
	}
}

// RunLogic is never called for a PASSIVE logic.
func (w *inactivityMonitor) RunLogic(types.Message) error {
	return errNotInvocable
}
