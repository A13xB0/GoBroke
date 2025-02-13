package GoBroke

import (
	"context"

	"github.com/A13xB0/GoBroke/types"
)

type LogicBase struct {
	name      string
	logicType types.LogicType
	Ctx       context.Context
	*Broke
}

func NewLogicBase(name string, logicType types.LogicType, broke *Broke) LogicBase {
	return LogicBase{
		name:      name,
		logicType: logicType,
		Broke:     broke,
		Ctx:       context.WithoutCancel(broke.ctx),
	}
}

func (w LogicBase) Type() types.LogicType {
	return w.logicType
}

func (w LogicBase) Name() string {
	return w.name
}
