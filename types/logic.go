package types

type LogicType int

const (
	DISPATCHED LogicType = iota
	WORKER
	PASSIVE
)

type Logic interface {
	RunLogic(Message) error
	Type() LogicType
	Name() string
}
