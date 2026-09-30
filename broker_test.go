package GoBroke_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/endpoint"
	brokeerrors "github.com/A13xB0/GoBroke/errors"
	"github.com/A13xB0/GoBroke/message"
	"github.com/A13xB0/GoBroke/types"
)

// testEndpoint records outbound messages. Like a real endpoint it stops
// reading the outbound channel when the broker's context ends (the broker
// never closes it).
type testEndpoint struct {
	in    chan types.Message
	outCh chan types.Message
	mu    sync.Mutex
	out   []types.Message
	bound *GoBroke.Broke
}

func (e *testEndpoint) Sender(ch chan types.Message) error   { e.outCh = ch; return nil }
func (e *testEndpoint) Receiver(ch chan types.Message) error { e.in = ch; return nil }
func (e *testEndpoint) Disconnect(*clients.Client) error     { return nil }
func (e *testEndpoint) Bind(b *GoBroke.Broke)                { e.bound = b }

func (e *testEndpoint) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case m := <-e.outCh:
				e.mu.Lock()
				e.out = append(e.out, m)
				e.mu.Unlock()
			}
		}
	}()
}

func (e *testEndpoint) sent() []types.Message {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]types.Message(nil), e.out...)
}

// fn is a logic whose behaviour is a closure.
type fn struct {
	GoBroke.LogicBase
	run func(types.Message) error
}

func (l *fn) RunLogic(m types.Message) error { return l.run(m) }

func newFn(b *GoBroke.Broke, name string, t types.LogicType, run func(types.Message), opts ...GoBroke.LogicOption) *fn {
	return &fn{GoBroke.NewLogicBase(types.LogicName(name), t, b, opts...), func(m types.Message) error { run(m); return nil }}
}

// runner is a PASSIVE logic with a Run loop.
type runner struct {
	GoBroke.LogicBase
	run func(ctx context.Context) error
}

func (r *runner) RunLogic(types.Message) error  { return nil }
func (r *runner) Run(ctx context.Context) error { return r.run(ctx) }

// harness runs a broker until the test stops it.
type harness struct {
	b      *GoBroke.Broke
	ep     *testEndpoint
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

func newHarness(t *testing.T, opts ...GoBroke.Option) *harness {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	ep := &testEndpoint{}
	b, err := GoBroke.New(ep, append([]GoBroke.Option{GoBroke.WithContext(ctx)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return &harness{b: b, ep: ep, ctx: ctx, cancel: cancel, done: make(chan struct{})}
}

func (h *harness) start() {
	go func() {
		h.b.Start()
		close(h.done)
	}()
}

func (h *harness) stop() {
	h.cancel()
	<-h.done
}

func (h *harness) add(t *testing.T, l types.Logic) {
	t.Helper()
	if err := h.b.AddLogic(l); err != nil {
		t.Fatal(err)
	}
}

func to(names ...string) []types.LogicName {
	out := make([]types.LogicName, len(names))
	for i, n := range names {
		out[i] = types.LogicName(n)
	}
	return out
}

func logicMsg(n int, names ...string) types.Message {
	return message.NewLogicMessage("test", nil, to(names...), nil, message.WithMetadata(map[string]any{"n": n}))
}

func clientMsg(c *clients.Client, n int, names ...string) types.Message {
	return message.NewClientMessage(c, nil, to(names...), nil, message.WithMetadata(map[string]any{"n": n}))
}

// concurrency tracks how many calls overlap.
type concurrency struct {
	running, max atomic.Int32
}

func (c *concurrency) enter() {
	if r := c.running.Add(1); r > c.max.Load() {
		c.max.Store(r)
	}
}

func (c *concurrency) leave() { c.running.Add(-1) }

func num(m types.Message) int { return m.Metadata["n"].(int) }

// Regression for review finding GB-1: shutdown closed the queues while
// producers still wrote to them ("send on closed channel").
func TestSendAfterShutdownDoesNotPanic(t *testing.T) {
	h := newHarness(t)
	h.add(t, newFn(h.b, "w", types.WORKER, func(types.Message) {}))
	h.start()
	h.stop()

	h.b.SendMessage(logicMsg(1, "w"))
	h.b.SendMessageQuickly(message.NewLogicMessage("x", []*clients.Client{clients.New()}, nil, nil))
	if err := h.b.Deliver(message.NewLogicMessage("x", []*clients.Client{clients.New()}, nil, nil)); !errors.Is(err, brokeerrors.ErrorBrokerNotRunning) {
		t.Fatalf("Deliver after shutdown: want ErrorBrokerNotRunning, got %v", err)
	}
}

// Regression for GB-2: a WORKER that sends to itself used to deadlock the
// router once the queue filled.
func TestWorkerSendingToItselfDoesNotDeadlock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, GoBroke.WithLaneLimit(1))
		var ran atomic.Int32
		var w *fn
		w = newFn(h.b, "echo", types.WORKER, func(m types.Message) {
			ran.Add(1)
			if num(m) == 0 {
				for i := 1; i <= 5; i++ {
					w.SendMessage(logicMsg(i, "echo"))
				}
			}
		})
		h.add(t, w)
		h.start()
		h.b.SendMessage(logicMsg(0, "echo"))
		synctest.Wait()
		if ran.Load() != 6 {
			t.Fatalf("ran %d, want 6", ran.Load())
		}
		h.stop()
	})
}

// Regression for GB-2 / the 1 s head-of-line result in the review: a busy
// WORKER must not delay an unrelated logic.
func TestBusyWorkerDoesNotDelayOtherLogics(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.add(t, newFn(h.b, "slow", types.WORKER, func(types.Message) { time.Sleep(200 * time.Millisecond) }))
		var pingAt time.Time
		h.add(t, newFn(h.b, "ping", types.DISPATCHED, func(types.Message) { pingAt = time.Now() }))
		h.start()
		start := time.Now()
		for i := range 10 {
			h.b.SendMessage(logicMsg(i, "slow"))
		}
		h.b.SendMessage(logicMsg(0, "ping"))
		synctest.Wait()
		if d := pingAt.Sub(start); d != 0 {
			t.Fatalf("ping waited %v behind the WORKER", d)
		}
		time.Sleep(3 * time.Second)
		h.stop()
	})
}

func TestWorkerRunsOneAtATimeInOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		var got []int
		var c concurrency
		h.add(t, newFn(h.b, "w", types.WORKER, func(m types.Message) {
			c.enter()
			defer c.leave()
			time.Sleep(time.Millisecond)
			got = append(got, num(m))
		}))
		h.start()
		for i := range 100 {
			h.b.SendMessage(logicMsg(i, "w"))
		}
		time.Sleep(time.Second)
		synctest.Wait()
		assertInOrder(t, got, 100)
		if c.max.Load() != 1 {
			t.Fatalf("up to %d ran at once", c.max.Load())
		}
		h.stop()
	})
}

func assertInOrder(t *testing.T, got []int, want int) {
	t.Helper()
	if len(got) != want {
		t.Fatalf("ran %d of %d", len(got), want)
	}
	for i, n := range got {
		if n != i {
			t.Fatalf("position %d ran message %d", i, n)
		}
	}
}

// Logics sharing a queue must run one at a time with each other, in arrival
// order — the "market" pattern from the mir1 mapping.
func TestInQueueSerialisesAcrossLogics(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		var c concurrency
		var order []string
		record := func(name string) func(types.Message) {
			return func(types.Message) {
				c.enter()
				defer c.leave()
				time.Sleep(time.Millisecond)
				order = append(order, name)
			}
		}
		h.add(t, newFn(h.b, "market.buy", types.WORKER, record("buy"), GoBroke.InQueue("market")))
		h.add(t, newFn(h.b, "market.expire", types.WORKER, record("expire"), GoBroke.InQueue("market")))
		h.start()
		for i := range 10 {
			h.b.SendMessage(logicMsg(i, "market.buy"))
			h.b.SendMessage(logicMsg(i, "market.expire"))
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if c.max.Load() != 1 {
			t.Fatalf("logics in one queue overlapped (%d at once)", c.max.Load())
		}
		assertAlternates(t, order, "buy", "expire")
		h.stop()
	})
}

func assertAlternates(t *testing.T, order []string, a, b string) {
	t.Helper()
	for i, name := range order {
		want := a
		if i%2 == 1 {
			want = b
		}
		if name != want {
			t.Fatalf("position %d ran %s, want %s", i, name, want)
		}
	}
}

// perKey records message numbers per map key.
type perKey struct {
	mu  sync.Mutex
	got map[string][]int
}

func (p *perKey) add(m types.Message) {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := m.Metadata["map"].(string)
	p.got[k] = append(p.got[k], num(m))
}

func (p *perKey) total() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, v := range p.got {
		n += len(v)
	}
	return n
}

// A queue keyed by map runs each map in order and different maps in parallel.
func TestKeyedByOrdersPerKeyAndRunsKeysInParallel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		byMap := func(m types.Message) string { return m.Metadata["map"].(string) }
		rec := &perKey{got: map[string][]int{}}
		h.add(t, newFn(h.b, "walk", types.WORKER, func(m types.Message) {
			time.Sleep(20 * time.Millisecond)
			rec.add(m)
		}, GoBroke.InQueue("world"), GoBroke.KeyedBy(byMap, 64)))
		h.start()
		maps := []string{"m1", "m2", "m3", "m4"}
		start := time.Now()
		for i := range 5 {
			for _, mp := range maps {
				h.b.SendMessage(message.NewLogicMessage("t", nil, to("walk"), nil,
					message.WithMetadata(map[string]any{"n": i, "map": mp})))
			}
		}
		for rec.total() < 20 {
			time.Sleep(time.Millisecond)
		}
		elapsed := time.Since(start)
		for _, mp := range maps {
			assertInOrder(t, rec.got[mp], 5)
		}
		// 5 steps x 20 ms per map. Serial would be 400 ms; allow lane collisions.
		if elapsed >= 400*time.Millisecond {
			t.Fatalf("keys did not run in parallel: %v", elapsed)
		}
		h.stop()
	})
}

func TestKeyedByShardConflictIsAnError(t *testing.T) {
	h := newHarness(t)
	key := func(types.Message) string { return "" }
	h.add(t, newFn(h.b, "a", types.WORKER, func(types.Message) {}, GoBroke.InQueue("world"), GoBroke.KeyedBy(key, 8)))
	err := h.b.AddLogic(newFn(h.b, "b", types.WORKER, func(types.Message) {}, GoBroke.InQueue("world"), GoBroke.KeyedBy(key, 16)))
	if !errors.Is(err, brokeerrors.ErrorQueueConflict) {
		t.Fatalf("want ErrorQueueConflict, got %v", err)
	}
	// Joining without KeyedBy uses the queue as declared.
	h.add(t, newFn(h.b, "c", types.WORKER, func(types.Message) {}, GoBroke.InQueue("world")))
}

// Client traffic waits at the edge when a lane is full; internal traffic never does.
func TestReceiveWaitsWhenLaneIsFull(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, GoBroke.WithLaneLimit(2))
		gate := make(chan struct{})
		h.add(t, newFn(h.b, "w", types.WORKER, func(types.Message) { <-gate }))
		h.start()
		c := clients.New()
		for i := range 2 {
			if err := h.b.Receive(t.Context(), clientMsg(c, i, "w")); err != nil {
				t.Fatal(err)
			}
		}
		synctest.Wait()

		third := make(chan error, 1)
		go func() { third <- h.b.Receive(t.Context(), clientMsg(c, 2, "w")) }()
		synctest.Wait()
		select {
		case <-third:
			t.Fatal("Receive returned while the lane was full")
		default:
		}

		h.b.SendMessage(logicMsg(99, "w")) // internal traffic is never held back: depth is now 3
		gate <- struct{}{}                 // depth 2: still at the limit
		gate <- struct{}{}                 // depth 1: the waiting client message is admitted
		synctest.Wait()
		if err := <-third; err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := h.b.Receive(ctx, clientMsg(c, 3, "w")); err == nil {
			t.Fatal("Receive on a full lane with a cancelled ctx should fail")
		}
		close(gate)
		h.stop()
	})
}

func TestTickCoalescesWhileBusy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		gate := make(chan struct{})
		var ticks atomic.Int32
		h.add(t, newFn(h.b, "w", types.WORKER, func(m types.Message) {
			if _, ok := GoBroke.TickKey(m); ok {
				ticks.Add(1)
				return
			}
			<-gate
		}))
		h.start()
		h.b.SendMessage(logicMsg(0, "w"))
		synctest.Wait()
		queued := 0
		for range 20 {
			queued += h.b.Tick("w")
		}
		if queued != 1 {
			t.Fatalf("queued %d ticks, want 1", queued)
		}
		close(gate)
		synctest.Wait()
		if ticks.Load() != 1 {
			t.Fatalf("ran %d ticks, want 1", ticks.Load())
		}
		if h.b.Tick("missing") != 0 {
			t.Fatal("Tick for an unknown logic should queue nothing")
		}
		h.stop()
	})
}

func TestTickForTargetsOneKey(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		byMap := func(m types.Message) string { return m.Metadata["map"].(string) }
		var mu sync.Mutex
		var keys []string
		h.add(t, newFn(h.b, "world.tick", types.WORKER, func(m types.Message) {
			if k, ok := GoBroke.TickKey(m); ok {
				mu.Lock()
				keys = append(keys, k)
				mu.Unlock()
			}
		}, GoBroke.InQueue("world"), GoBroke.KeyedBy(byMap, 16)))
		h.start()
		if !h.b.TickFor("world.tick", "m7") || !h.b.TickFor("world.tick", "m9") {
			t.Fatal("TickFor should queue")
		}
		synctest.Wait()
		if len(keys) != 2 {
			t.Fatalf("ran ticks for %v", keys)
		}
		if h.b.TickFor("missing", "m1") {
			t.Fatal("TickFor on an unknown logic should not queue")
		}
		h.stop()
	})
}

func TestTickDispatchedCoalesces(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		gate := make(chan struct{})
		var ran atomic.Int32
		h.add(t, newFn(h.b, "d", types.DISPATCHED, func(types.Message) { ran.Add(1); <-gate }))
		h.start()
		if h.b.Tick("d") != 1 {
			t.Fatal("first tick should queue")
		}
		synctest.Wait() // the tick is running (flag cleared), blocked on gate
		if h.b.Tick("d") != 1 {
			t.Fatal("a tick should queue once the previous one started")
		}
		close(gate)
		synctest.Wait()
		if ran.Load() != 2 {
			t.Fatalf("ran %d, want 2", ran.Load())
		}
		h.stop()
	})
}

// Runner loops start with the broker and stop when it shuts down.
func TestRunnerLifecycle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		started := make(chan struct{})
		stopped := make(chan struct{})
		h.add(t, &runner{GoBroke.NewLogicBase("clock", types.PASSIVE, h.b), func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			close(stopped)
			return nil
		}})
		h.start()
		<-started
		h.stop()
		select {
		case <-stopped:
		default:
			t.Fatal("Start returned before the Runner stopped")
		}
	})
}

func TestRunnerAddedAfterStartIsStarted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.start()
		synctest.Wait()
		started := make(chan struct{})
		h.add(t, &runner{GoBroke.NewLogicBase("late", types.PASSIVE, h.b), func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return nil
		}})
		<-started
		h.stop()
	})
}

// Regression for GB-6: LogicBase.Ctx used context.WithoutCancel and never ended.
func TestLogicContextEndsOnShutdown(t *testing.T) {
	h := newHarness(t)
	l := newFn(h.b, "w", types.WORKER, func(types.Message) {})
	h.add(t, l)
	h.start()
	h.stop()
	select {
	case <-l.Ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("LogicBase.Ctx was not cancelled on shutdown")
	}
}

func TestPassiveLogicReceivesNoMessages(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		var ran atomic.Int32
		h.add(t, newFn(h.b, "p", types.PASSIVE, func(types.Message) { ran.Add(1) }))
		h.start()
		h.b.SendMessage(logicMsg(0, "p"))
		synctest.Wait()
		if ran.Load() != 0 {
			t.Fatal("PASSIVE logic received a message")
		}
		h.stop()
	})
}

func TestMaxConcurrentCapsClientTraffic(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		gate := make(chan struct{})
		var c concurrency
		h.add(t, newFn(h.b, "d", types.DISPATCHED, func(types.Message) {
			c.enter()
			defer c.leave()
			<-gate
		}, GoBroke.MaxConcurrent(2)))
		h.start()
		sender := clients.New()
		for i := range 5 {
			go func() { _ = h.b.Receive(t.Context(), clientMsg(sender, i, "d")) }()
		}
		synctest.Wait()
		if c.running.Load() != 2 {
			t.Fatalf("%d running, want 2", c.running.Load())
		}
		close(gate)
		synctest.Wait()
		if c.max.Load() != 2 {
			t.Fatalf("up to %d ran at once, want 2", c.max.Load())
		}
		h.stop()
	})
}

func TestReceiveMiddlewareRejectDropsMessage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		var ran atomic.Int32
		h.add(t, newFn(h.b, "w", types.WORKER, func(types.Message) { ran.Add(1) }))
		h.b.AttachReceiveMiddleware(func(m types.Message) types.Message {
			if num(m) == 1 {
				m.Reject()
			}
			return m
		})
		h.start()
		c := clients.New()
		if err := h.b.Receive(t.Context(), clientMsg(c, 1, "w")); !errors.Is(err, brokeerrors.ErrorMessageRejected) {
			t.Fatalf("want ErrorMessageRejected, got %v", err)
		}
		if err := h.b.Receive(t.Context(), clientMsg(c, 2, "w")); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if ran.Load() != 1 {
			t.Fatalf("ran %d, want 1", ran.Load())
		}
		h.stop()
	})
}

// Regression for GB-7: the router reset State to ACCEPTED before middleware,
// and send middleware could not reject at all.
func TestRejectedStateIsHonoured(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		var ran atomic.Int32
		h.add(t, newFn(h.b, "w", types.WORKER, func(types.Message) { ran.Add(1) }))
		h.b.AttachSendMiddleware(func(m types.Message) types.Message {
			if string(m.MessageRaw) == "drop" {
				m.Reject()
			}
			return m
		})
		h.start()
		m := logicMsg(0, "w")
		m.Reject()
		h.b.SendMessage(m)

		recipient := []*clients.Client{clients.New()}
		h.b.SendMessageQuickly(message.NewLogicMessage("x", recipient, nil, []byte("drop")))
		if err := h.b.Deliver(message.NewLogicMessage("x", recipient, nil, []byte("drop"))); !errors.Is(err, brokeerrors.ErrorMessageRejected) {
			t.Fatalf("Deliver: want ErrorMessageRejected, got %v", err)
		}
		if err := h.b.Deliver(message.NewLogicMessage("x", recipient, nil, []byte("keep"))); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if ran.Load() != 0 {
			t.Fatal("a REJECTED message reached logic")
		}
		if sent := h.ep.sent(); len(sent) != 1 || string(sent[0].MessageRaw) != "keep" {
			t.Fatalf("endpoint got %d messages, want only the kept one", len(sent))
		}
		h.stop()
	})
}

// Regression for the missing continue: a SentQuickly message arriving on the
// inbound channel was delivered and also run by logic.
func TestSentQuicklyMessagesSkipLogic(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		var ran atomic.Int32
		h.add(t, newFn(h.b, "w", types.WORKER, func(types.Message) { ran.Add(1) }))
		h.start()
		m := message.NewLogicMessage("x", []*clients.Client{clients.New()}, to("w"), nil)
		m.SentQuickly = true
		h.ep.in <- m
		synctest.Wait()
		if ran.Load() != 0 {
			t.Fatal("SentQuickly message was run by logic")
		}
		if len(h.ep.sent()) != 1 {
			t.Fatal("SentQuickly message was not delivered")
		}
		h.stop()
	})
}

// Endpoints that still write to the Receiver channel keep working.
func TestLegacyReceiverChannelIsRouted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		var ran atomic.Int32
		h.add(t, newFn(h.b, "w", types.WORKER, func(types.Message) { ran.Add(1) }))
		h.start()
		h.ep.in <- clientMsg(clients.New(), 0, "w")
		synctest.Wait()
		if ran.Load() != 1 {
			t.Fatal("message on the Receiver channel was not routed")
		}
		h.stop()
	})
}

func TestNewCallsBind(t *testing.T) {
	h := newHarness(t)
	if h.ep.bound != h.b {
		t.Fatal("New did not call Bind on a Binder endpoint")
	}
}

// Regression for GB-5: StubEndpoint.Start blocks, and the broker used to call
// it inline, so nothing was ever routed.
func TestBlockingEndpointStartDoesNotStopRouting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		b, err := GoBroke.New(endpoint.NewStubEndpoint(), GoBroke.WithContext(ctx))
		if err != nil {
			t.Fatal(err)
		}
		var ran atomic.Int32
		if err := b.AddLogic(newFn(b, "w", types.WORKER, func(types.Message) { ran.Add(1) })); err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() { b.Start(); close(done) }()
		b.SendMessage(logicMsg(0, "w"))
		synctest.Wait()
		if ran.Load() != 1 {
			t.Fatal("message was not routed while the endpoint's Start blocked")
		}
		cancel()
		<-done
	})
}

func TestPanicsAreRecoveredInEveryKind(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex
		var panicked []types.LogicName
		h := newHarness(t, GoBroke.WithOnLogicPanic(func(name types.LogicName, _ types.Message, _ any, _ string) {
			mu.Lock()
			panicked = append(panicked, name)
			mu.Unlock()
		}))
		var after atomic.Int32
		h.add(t, newFn(h.b, "w", types.WORKER, func(m types.Message) {
			if num(m) == 0 {
				panic("worker")
			}
			after.Add(1)
		}))
		h.add(t, newFn(h.b, "d", types.DISPATCHED, func(types.Message) { panic("dispatched") }))
		h.add(t, &runner{GoBroke.NewLogicBase("r", types.PASSIVE, h.b), func(context.Context) error { panic("runner") }})
		h.start()
		h.b.SendMessage(logicMsg(0, "w"))
		h.b.SendMessage(logicMsg(1, "w"))
		h.b.SendMessage(logicMsg(0, "d"))
		synctest.Wait()
		if len(panicked) != 3 {
			t.Fatalf("recovered %v, want w, d and r", panicked)
		}
		if after.Load() != 1 {
			t.Fatal("the WORKER lane stopped after a panic")
		}
		h.stop()
	})
}

// Regression for GB-8: RunLogic errors used to be discarded.
func TestLogicErrorsAreLogged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var buf bytes.Buffer
		var mu sync.Mutex
		logger := slog.New(slog.NewTextHandler(&lockedWriter{w: &buf, mu: &mu}, nil))
		h := newHarness(t, GoBroke.WithLogger(logger))
		h.add(t, &fn{GoBroke.NewLogicBase("w", types.WORKER, h.b), func(types.Message) error { return errors.New("boom") }})
		h.start()
		h.b.SendMessage(logicMsg(0, "w"))
		synctest.Wait()
		h.stop()
		mu.Lock()
		defer mu.Unlock()
		if !strings.Contains(buf.String(), "boom") {
			t.Fatalf("error not logged: %q", buf.String())
		}
	})
}

type lockedWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func TestAddAndRemoveLogic(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		var ran atomic.Int32
		l := newFn(h.b, "w", types.WORKER, func(types.Message) { ran.Add(1) })
		h.add(t, l)
		if err := h.b.AddLogic(l); !errors.Is(err, brokeerrors.ErrorLogicAlreadyExists) {
			t.Fatalf("want ErrorLogicAlreadyExists, got %v", err)
		}
		h.start()
		if err := h.b.RemoveLogic("w"); err != nil {
			t.Fatal(err)
		}
		if err := h.b.RemoveLogic("w"); err != nil {
			t.Fatal("removing a missing logic should not fail")
		}
		h.b.SendMessage(logicMsg(0, "w"))
		synctest.Wait()
		if ran.Load() != 0 {
			t.Fatal("removed logic still received messages")
		}
		h.stop()
	})
}

// Regression for the RegisterClient check-then-insert race.
func TestConcurrentRegisterClientAcceptsOne(t *testing.T) {
	h := newHarness(t)
	var ok atomic.Int32
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if h.b.RegisterClient(clients.New(clients.WithUUID("same"))) == nil {
				ok.Add(1)
			}
		})
	}
	wg.Wait()
	if ok.Load() != 1 {
		t.Fatalf("%d registrations succeeded, want 1", ok.Load())
	}
}
