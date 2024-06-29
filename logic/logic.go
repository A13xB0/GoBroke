package logic

import (
	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/message"
)

type LogicType int

type NewLogic func(*GoBroke.Broke) error

const (
	DISPATCHED LogicType = iota
	WORKER
)

type Logic interface {
	RunLogic(message.Message) error
	Type() LogicType
	Name() string
}
