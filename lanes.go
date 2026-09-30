package GoBroke

import (
	"context"
	"hash/maphash"
	"sync"
	"sync/atomic"

	"github.com/A13xB0/GoBroke/types"
)

// defaultLaneLimit is how many messages a lane may hold before a client
// message arriving through Receive waits for space.
const defaultLaneLimit = 1024

// job is one message for one logic, waiting in a lane.
type job struct {
	entry *logicEntry
	msg   types.Message
	tick  *atomic.Bool // set for coalesced ticks; cleared when the tick runs
}

// lane runs jobs one at a time, in the order they were pushed.
//
// push never blocks, so a logic may safely push into its own lane. Limits are
// applied only at the edge: admit waits while the lane is at its limit. That
// stalls only the calling connection's goroutine, never the lane itself.
//
// The lane goroutine takes all pending jobs in one lock and reuses the batch's
// backing array, so a steady stream costs one lock per push and no allocation.
type lane struct {
	mu    sync.Mutex
	q     []job         // pending jobs, appended by push
	spare []job         // the last processed batch, reused as the next q
	idle  atomic.Bool   // the lane goroutine is waiting (or about to wait) for work
	wake  chan struct{} // capacity 1: "there may be work"
	space chan struct{} // closed and replaced whenever depth drops below limit
	depth atomic.Int64
	limit int64

	ticks sync.Map // tick key -> *atomic.Bool
}

func newLane(limit int) *lane {
	if limit < 1 {
		limit = defaultLaneLimit
	}
	return &lane{
		wake:  make(chan struct{}, 1),
		space: make(chan struct{}),
		limit: int64(limit),
	}
}

func (l *lane) push(j job) {
	l.depth.Add(1)
	l.mu.Lock()
	l.q = append(l.q, j)
	l.mu.Unlock()
	// Only a sleeping lane needs waking; a busy one picks the job up in its
	// next batch.
	if l.idle.Load() && l.idle.CompareAndSwap(true, false) {
		select {
		case l.wake <- struct{}{}:
		default:
		}
	}
}

// admit waits until the lane is below its limit or ctx is done.
func (l *lane) admit(ctx context.Context) error {
	for l.depth.Load() >= l.limit {
		l.mu.Lock()
		space := l.space
		l.mu.Unlock()
		select {
		case <-space:
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
	return nil
}

// tick pushes a tick job unless one for the same key is already waiting.
func (l *lane) tick(key string, j job) bool {
	v, _ := l.ticks.LoadOrStore(key, new(atomic.Bool))
	pending := v.(*atomic.Bool)
	if !pending.CompareAndSwap(false, true) {
		return false
	}
	j.tick = pending
	l.push(j)
	return true
}

// take swaps out every pending job in one lock. Hand the batch back with
// recycle once it has been processed.
func (l *lane) take() []job {
	l.mu.Lock()
	defer l.mu.Unlock()
	batch := l.q
	l.q = l.spare
	l.spare = nil
	return batch
}

// recycle keeps a processed batch's backing array for reuse, unless a burst
// made it unusually large, so memory is returned after spikes.
func (l *lane) recycle(batch []job) {
	if batch == nil || cap(batch) > 4*int(l.limit) {
		return
	}
	clear(batch)
	l.mu.Lock()
	if l.spare == nil {
		l.spare = batch[:0]
	}
	l.mu.Unlock()
}

// finished records that n jobs completed and wakes waiting admitters if the
// lane dropped below its limit.
func (l *lane) finished(n int) {
	if n == 0 {
		return
	}
	after := l.depth.Add(-int64(n))
	if after < l.limit && after+int64(n) >= l.limit {
		l.mu.Lock()
		close(l.space)
		l.space = make(chan struct{})
		l.mu.Unlock()
	}
}

// wait blocks until there may be work or done is closed. It reports false
// when done is closed.
func (l *lane) wait(done <-chan struct{}) bool {
	l.idle.Store(true)
	// Re-check after announcing idle so a push racing with us isn't missed.
	l.mu.Lock()
	pending := len(l.q) > 0
	l.mu.Unlock()
	if pending {
		l.idle.Store(false)
		return true
	}
	select {
	case <-done:
		return false
	case <-l.wake:
		return true
	}
}

// run processes jobs until ctx is done. Jobs still queued at shutdown are
// dropped; the returned count says how many.
func (l *lane) run(ctx context.Context, exec func(job)) int {
	done := ctx.Done()
	for {
		batch := l.take()
		if len(batch) == 0 {
			l.recycle(batch)
			if !l.wait(done) {
				return int(l.depth.Load())
			}
			continue
		}
		for _, j := range batch {
			select {
			case <-done:
				return int(l.depth.Load()) // finished() already counted the jobs that ran
			default:
			}
			if j.tick != nil {
				j.tick.Store(false)
			}
			exec(j)
			l.finished(1)
		}
		l.recycle(batch)
	}
}

// queue is a named group of lanes. Logics that share a queue run one at a
// time with each other; with a key function, messages with the same key share
// a lane (in order) and different keys may run in parallel.
type queue struct {
	name   string
	key    KeyFunc
	seed   maphash.Seed
	lanes  []*lane
	keyed  bool
	shards int
}

func newQueue(name string, key KeyFunc, shards, limit int) *queue {
	q := &queue{name: name, key: key, seed: maphash.MakeSeed(), keyed: key != nil, shards: shards}
	if !q.keyed || shards < 1 {
		q.shards = 1
	}
	q.lanes = make([]*lane, q.shards)
	for i := range q.lanes {
		q.lanes[i] = newLane(limit)
	}
	return q
}

func (q *queue) laneForKey(key string) *lane {
	if len(q.lanes) == 1 {
		return q.lanes[0]
	}
	return q.lanes[maphash.String(q.seed, key)%uint64(len(q.lanes))]
}

func (q *queue) laneFor(m types.Message) *lane {
	if key, ok := TickKey(m); ok {
		return q.laneForKey(key)
	}
	if !q.keyed {
		return q.lanes[0]
	}
	return q.laneForKey(q.key(m))
}
