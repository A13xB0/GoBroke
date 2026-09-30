package GoBroke

import (
	"strconv"

	"github.com/A13xB0/GoBroke/types"
)

// TickSource is the FromLogic of messages created by Tick and TickFor.
const TickSource types.LogicName = "gobroke.tick"

// tickKeyTag carries the lane key of a tick message.
const tickKeyTag = "gobroke.tick.key"

// Tick asks a logic to run once with a tick message, without piling up work:
// if a tick for the logic is already waiting, this one is skipped. Clocks
// (PASSIVE logics with Run) use it to hand time-driven work to the WORKER
// that owns the state, so the work runs in order with that state's messages.
//
// For a keyed WORKER queue a tick goes to every lane; use TickFor to target
// one key. It returns how many ticks were queued.
func (broke *Broke) Tick(name types.LogicName) int {
	e, ok := broke.lookup(name)
	if !ok {
		return 0
	}
	switch e.logic.Type() {
	case types.WORKER:
		queued := 0
		for i, l := range e.queue.lanes {
			if l.tick(string(name)+"\x01lane"+strconv.Itoa(i), job{entry: e, msg: tickMessage(name, "")}) {
				queued++
			}
		}
		return queued
	case types.DISPATCHED:
		if e.dispatchTick.CompareAndSwap(false, true) {
			broke.dispatch(e, tickMessage(name, ""), &e.dispatchTick)
			return 1
		}
	}
	return 0
}

// TickFor sends a coalesced tick for one key of a keyed WORKER queue, for
// example one map of a world queue keyed by map. It reports whether the tick
// was queued (false if one for the same logic and key is already waiting).
func (broke *Broke) TickFor(name types.LogicName, key string) bool {
	e, ok := broke.lookup(name)
	if !ok || e.logic.Type() != types.WORKER {
		return false
	}
	l := e.queue.laneForKey(key)
	return l.tick(string(name)+"\x00"+key, job{entry: e, msg: tickMessage(name, key)})
}

// TickKey reports whether m is a tick created by Tick or TickFor, and the key
// it was sent for ("" for Tick).
func TickKey(m types.Message) (string, bool) {
	if m.FromLogic != TickSource {
		return "", false
	}
	k, _ := m.Tags[tickKeyTag].(string)
	return k, true
}

func tickMessage(name types.LogicName, key string) types.Message {
	m := types.Message{
		ToLogic:   []types.LogicName{name},
		FromLogic: TickSource,
		Tags:      map[string]any{tickKeyTag: key},
	}
	if key == "" {
		m.Tags = nil
	}
	return m
}
