// Package GoBroke provides the base implementation for logic handlers
// in the GoBroke message broker system.
package GoBroke

import (
	"context"

	"github.com/A13xB0/GoBroke/types"
)

// KeyFunc picks the ordering key for a message in a keyed WORKER queue,
// for example the map or guild of the sender. Messages with the same key run
// one at a time, in order; different keys may run in parallel.
type KeyFunc func(types.Message) string

// Runner is optional for a logic of any type: a background loop that the
// broker starts in Start and stops by cancelling ctx on shutdown. A PASSIVE
// logic with Run is the usual shape for clocks and simulation ticks.
type Runner interface {
	Run(ctx context.Context) error
}

// logicOpts configures how the broker runs one logic.
type logicOpts struct {
	queue         string  // WORKER: shared queue name ("" = own queue)
	key           KeyFunc // WORKER: optional ordering key
	shards        int     // WORKER: lanes for a keyed queue
	maxConcurrent int     // DISPATCHED: cap on concurrent RunLogic calls (0 = none)
}

// LogicOption configures how the broker runs a logic. Pass options to
// NewLogicBase next to the logic type.
type LogicOption func(*logicOpts)

// InQueue makes WORKER logics that share a queue name run one at a time with
// each other, in arrival order. Use it when several logics change the same
// state (for example every market packet plus the auction-expiry tick).
func InQueue(name string) LogicOption {
	return func(o *logicOpts) { o.queue = name }
}

// KeyedBy splits a WORKER queue into shards lanes. Messages whose key is
// equal always share a lane and run in order; different keys run in parallel.
// Two keys that hash to the same lane only share it, which costs parallelism
// but never ordering. Every logic in a shared queue must use the same shard
// count; the first one registered supplies the key function.
func KeyedBy(fn KeyFunc, shards int) LogicOption {
	return func(o *logicOpts) {
		o.key = fn
		o.shards = shards
	}
}

// MaxConcurrent caps how many RunLogic calls of a DISPATCHED logic run at
// once for messages arriving through Receive (client traffic); further client
// messages wait at the edge. Messages sent by other logics are never held back,
// so a logic can't deadlock on its own cap.
func MaxConcurrent(n int) LogicOption {
	return func(o *logicOpts) { o.maxConcurrent = n }
}

// LogicBase provides a base implementation of the types.Logic interface.
// It implements common functionality that can be embedded in specific logic handlers.
type LogicBase struct {
	name      types.LogicName // Unique name of the logic handler
	logicType types.LogicType // Type of logic handler (WORKER, DISPATCHED, or PASSIVE)
	opts      logicOpts
	// Ctx is cancelled when the broker's context ends, so background loops
	// started by the logic stop on shutdown.
	Ctx    context.Context
	*Broke // Embedded broker instance for accessing broker functionality
}

// NewLogicBase creates a new LogicBase instance with the specified configuration.
// Parameters:
//   - name: Unique identifier for the logic handler
//   - logicType: Determines how messages are processed (WORKER, DISPATCHED, or PASSIVE)
//   - broke: Reference to the broker instance
//   - opts: Optional scheduling options (InQueue, KeyedBy, MaxConcurrent)
func NewLogicBase(name types.LogicName, logicType types.LogicType, broke *Broke, opts ...LogicOption) LogicBase {
	lb := LogicBase{
		name:      name,
		logicType: logicType,
		Broke:     broke,
		Ctx:       broke.ctx,
	}
	for _, fn := range opts {
		fn(&lb.opts)
	}
	return lb
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

// scheduling returns the options passed to NewLogicBase. It is unexported so
// only logics built on LogicBase carry options; others use the defaults.
func (w LogicBase) scheduling() logicOpts {
	return w.opts
}

type scheduled interface {
	scheduling() logicOpts
}
