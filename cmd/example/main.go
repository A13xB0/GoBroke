package main

import (
	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/examples/logic/broadcaster"
)

func main() {
	gb := GoBroke.New(nil)

	//Add Logic
	broadcasterLogic := broadcaster.CreateDispatched(gb)
	err := gb.AddLogic(broadcasterLogic)
	if err != nil {
		panic(err)
	}

	err = gb.Start()
	if err != nil {
		panic(err)
	}
}
