package GoBroke

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/A13xB0/GoBroke/types"
)

func msgN(n int) types.Message {
	return types.Message{Metadata: map[string]any{"n": n}}
}

func TestLaneRunsInOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		l := newLane(10)
		var got []int
		done := make(chan struct{})
		go func() {
			l.run(ctx, func(j job) { got = append(got, j.msg.Metadata["n"].(int)) })
			close(done)
		}()
		for i := range 50 {
			l.push(job{msg: msgN(i)})
		}
		synctest.Wait()
		cancel()
		<-done
		for i, n := range got {
			if n != i {
				t.Fatalf("position %d ran message %d", i, n)
			}
		}
		if len(got) != 50 {
			t.Fatalf("ran %d of 50", len(got))
		}
	})
}

// Regression for review finding GB-2: a job that pushes into its own full
// lane must not deadlock.
func TestLaneSelfPushDoesNotDeadlock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		l := newLane(1)
		var ran atomic.Int32
		done := make(chan struct{})
		go func() {
			l.run(ctx, func(j job) {
				ran.Add(1)
				if j.msg.Metadata["n"].(int) == 0 {
					for i := 1; i <= 5; i++ {
						l.push(job{msg: msgN(i)})
					}
				}
			})
			close(done)
		}()
		l.push(job{msg: msgN(0)})
		synctest.Wait()
		if ran.Load() != 6 {
			t.Fatalf("ran %d jobs, want 6", ran.Load())
		}
		cancel()
		<-done
	})
}

func TestLaneAdmitWaitsForSpace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		l := newLane(2)
		gate := make(chan struct{})
		done := make(chan struct{})
		go func() {
			l.run(ctx, func(job) { <-gate })
			close(done)
		}()
		l.push(job{})
		l.push(job{})

		admitted := make(chan error, 1)
		go func() { admitted <- l.admit(t.Context()) }()
		synctest.Wait()
		select {
		case <-admitted:
			t.Fatal("admit returned while the lane was full")
		default:
		}

		gate <- struct{}{} // finish one job; depth drops below the limit
		synctest.Wait()
		if err := <-admitted; err != nil {
			t.Fatal(err)
		}
		close(gate)
		cancel()
		<-done
	})
}

func TestLaneAdmitStopsWhenContextEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newLane(1)
		l.push(job{}) // nothing runs the lane, so it stays full
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		if err := l.admit(ctx); err == nil {
			t.Fatal("admit on a full lane should fail when ctx ends")
		}
	})
}

func TestLaneTickCoalesces(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		l := newLane(100)
		gate := make(chan struct{})
		var ticks atomic.Int32
		done := make(chan struct{})
		go func() {
			l.run(ctx, func(j job) {
				if j.tick != nil {
					ticks.Add(1)
					return
				}
				<-gate
			})
			close(done)
		}()
		l.push(job{}) // busy
		synctest.Wait()
		queued := 0
		for range 20 {
			if l.tick("k", job{}) {
				queued++
			}
		}
		if queued != 1 {
			t.Fatalf("queued %d ticks while one was pending, want 1", queued)
		}
		close(gate)
		synctest.Wait()
		if ticks.Load() != 1 {
			t.Fatalf("ran %d ticks, want 1", ticks.Load())
		}
		if !l.tick("k", job{}) {
			t.Fatal("a new tick should queue once the previous one has run")
		}
		synctest.Wait()
		cancel()
		<-done
	})
}

func TestLaneStopDropsQueuedJobs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		l := newLane(100)
		gate := make(chan struct{})
		dropped := make(chan int, 1)
		go func() { dropped <- l.run(ctx, func(job) { <-gate }) }()
		for range 5 {
			l.push(job{})
		}
		synctest.Wait()
		cancel()
		close(gate)
		if n := <-dropped; n != 4 {
			t.Fatalf("dropped %d, want 4 (one was running)", n)
		}
	})
}

func TestQueueKeysMapToStableLanes(t *testing.T) {
	q := newQueue("world", func(m types.Message) string { return m.Metadata["k"].(string) }, 8, 10)
	key := func(k string) types.Message { return types.Message{Metadata: map[string]any{"k": k}} }
	firstLane, secondLane := q.laneFor(key("map-1")), q.laneFor(key("map-1"))
	if firstLane != secondLane {
		t.Fatal("same key must always use the same lane")
	}
	seen := map[*lane]bool{}
	for i := range 64 {
		seen[q.laneFor(key("map-"+string(rune('a'+i%26))+string(rune('a'+i/26))))] = true
	}
	if len(seen) < 4 {
		t.Fatalf("64 keys used only %d of 8 lanes", len(seen))
	}
	if newQueue("solo", nil, 8, 10).shards != 1 {
		t.Fatal("an unkeyed queue has one lane")
	}
}
