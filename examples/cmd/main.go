// Command example wires a broker to the in-memory stub endpoint with one
// logic of each type and runs until interrupted.
package main

import (
	"context"
	"os"
	"os/signal"
	"time"

	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/endpoint"
	"github.com/A13xB0/GoBroke/examples/logic/broadcaster"
	"github.com/A13xB0/GoBroke/examples/logic/inactivitymonitor"
	"github.com/A13xB0/GoBroke/types"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	gb, err := GoBroke.New(endpoint.NewStubEndpoint(), GoBroke.WithContext(ctx))
	if err != nil {
		panic(err)
	}

	mustAdd(gb, broadcaster.CreateDispatched(gb))
	mustAdd(gb, broadcaster.CreateWorker(gb))
	mustAdd(gb, inactivitymonitor.Create(gb, 15*time.Minute))

	gb.Start() // blocks until ctx is cancelled
}

func mustAdd(gb *GoBroke.Broke, l types.Logic) {
	if err := gb.AddLogic(l); err != nil {
		panic(err)
	}
}
