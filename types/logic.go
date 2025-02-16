// Package types provides core type definitions for the GoBroke message broker system.
package types

// LogicType represents the execution mode of a logic handler.
type LogicType int

// LogicName represents the name of the logic handler
type LogicName string

const (
	// DISPATCHED indicates the logic handler runs in a new goroutine for each message.
	DISPATCHED LogicType = iota

	// WORKER indicates the logic handler processes messages sequentially.
	WORKER

	// PASSIVE indicates the logic handler only observes messages without processing.
	PASSIVE
)

// Logic defines the interface that all logic handlers must implement.
// Logic handlers are responsible for processing messages within the GoBroke system.
type Logic interface {
	// RunLogic processes a message and returns an error if processing fails.
	RunLogic(Message) error

	// Type returns the LogicType that determines how this handler processes messages.
	Type() LogicType

	// Name returns a unique identifier for this logic handler.
	Name() LogicName
}
