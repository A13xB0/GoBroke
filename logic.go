// Package GoBroke provides the base implementation for logic handlers
// in the GoBroke message broker system.
package GoBroke

import (
	"context"

	"github.com/A13xB0/GoBroke/types"
)

// LogicBase provides a base implementation of the types.Logic interface.
// It implements common functionality that can be embedded in specific logic handlers.
type LogicBase struct {
	name      types.LogicName // Unique name of the logic handler
	logicType types.LogicType // Type of logic handler (WORKER, DISPATCHED, or PASSIVE)
	Ctx       context.Context // Context for cancellation and value propagation
	*Broke                    // Embedded broker instance for accessing broker functionality
}

// NewLogicBase creates a new LogicBase instance with the specified configuration.
// Parameters:
//   - name: Unique identifier for the logic handler
//   - logicType: Determines how messages are processed (WORKER, DISPATCHED, or PASSIVE)
//   - broke: Reference to the broker instance
//
// Returns a LogicBase configured with the provided parameters and a derived context.
func NewLogicBase(name types.LogicName, logicType types.LogicType, broke *Broke) LogicBase {
	return LogicBase{
		name:      name,
		logicType: logicType,
		Broke:     broke,
		Ctx:       context.WithoutCancel(broke.ctx),
	}
}

// Type returns the LogicType of this handler (WORKER, DISPATCHED, or PASSIVE).
// This method satisfies part of the types.Logic interface.
func (w LogicBase) Type() types.LogicType {
	return w.logicType
}

// Name returns the unique identifier of this logic handler.
// This method satisfies part of the types.Logic interface.
func (w LogicBase) Name() types.LogicName {
	return w.name
}
