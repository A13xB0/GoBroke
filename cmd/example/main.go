package main

import (
	"context"
	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/examples/logic/broadcaster"
	"github.com/A13xB0/GoBroke/examples/logic/inactivitymonitor"
)

func main() {
	ctx := context.Background()
	gb := GoBroke.New(nil, GoBroke.WithContext(ctx))

	//Add Logic
	broadcasterLogic := broadcaster.CreateDispatched(gb)
	_ = gb.AddLogic(broadcasterLogic)
	inactivityMonitor := inactivitymonitor.CreateWorker(gb, 15, ctx)
	_ = gb.AddLogic(inactivityMonitor)

	//Start GoBroke
	gb.Start()
}
