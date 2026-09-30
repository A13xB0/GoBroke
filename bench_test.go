package GoBroke_test

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/message"
	"github.com/A13xB0/GoBroke/types"
)

// These benchmarks only use the public API that has existed since v0.3.0,
// so the same file measures old and new implementations.

// benchEndpoint returns from Start immediately (like Nixie's endpoint) and
// drains the outbound channel.
type benchEndpoint struct {
	out  chan types.Message
	sent atomic.Int64
}

func (e *benchEndpoint) Sender(ch chan types.Message) error { e.out = ch; return nil }
func (e *benchEndpoint) Receiver(chan types.Message) error  { return nil }
func (e *benchEndpoint) Disconnect(*clients.Client) error   { return nil }
func (e *benchEndpoint) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-e.out:
				if !ok {
					return
				}
				e.sent.Add(1)
			}
		}
	}()
}

type fnLogic struct {
	GoBroke.LogicBase
	fn func(types.Message)
}

func (l *fnLogic) RunLogic(m types.Message) error { l.fn(m); return nil }

func newBroker(b *testing.B) (*GoBroke.Broke, *benchEndpoint, context.CancelFunc) {
	b.Helper()
	ep := &benchEndpoint{}
	ctx, cancel := context.WithCancel(context.Background())
	broke, err := GoBroke.New(ep, GoBroke.WithContext(ctx), GoBroke.WithChannelSize(1000))
	if err != nil {
		b.Fatal(err)
	}
	return broke, ep, cancel
}

func waitFor(b *testing.B, n *atomic.Int64, want int64) {
	b.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for n.Load() < want {
		if time.Now().After(deadline) {
			b.Fatalf("processed %d/%d", n.Load(), want)
		}
		time.Sleep(50 * time.Microsecond)
	}
}

func benchRoute(b *testing.B, lt types.LogicType) {
	broke, _, cancel := newBroker(b)
	var n atomic.Int64
	_ = broke.AddLogic(&fnLogic{GoBroke.NewLogicBase("sink", lt, broke), func(types.Message) { n.Add(1) }})
	go broke.Start()
	from := clients.New()
	to := []types.LogicName{"sink"}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		broke.SendMessage(message.NewClientMessage(from, nil, to, nil))
	}
	waitFor(b, &n, int64(b.N))
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "msgs/s")
	cancel()
}

// BenchmarkRouteDispatched: one sender → DISPATCHED logic.
func BenchmarkRouteDispatched(b *testing.B) { benchRoute(b, types.DISPATCHED) }

// BenchmarkRouteWorker: one sender → WORKER logic.
func BenchmarkRouteWorker(b *testing.B) { benchRoute(b, types.WORKER) }

// BenchmarkRouteParallelSenders: many goroutines (connections) → DISPATCHED.
func BenchmarkRouteParallelSenders(b *testing.B) {
	broke, _, cancel := newBroker(b)
	var n atomic.Int64
	_ = broke.AddLogic(&fnLogic{GoBroke.NewLogicBase("sink", types.DISPATCHED, broke), func(types.Message) { n.Add(1) }})
	go broke.Start()
	to := []types.LogicName{"sink"}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		from := clients.New()
		for pb.Next() {
			broke.SendMessage(message.NewClientMessage(from, nil, to, nil))
		}
	})
	waitFor(b, &n, int64(b.N))
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "msgs/s")
	cancel()
}

// BenchmarkDeliver: logic → clients path (SendMessageQuickly), 10 recipients.
func BenchmarkDeliver(b *testing.B) {
	broke, ep, cancel := newBroker(b)
	go broke.Start()
	to := make([]*clients.Client, 10)
	for i := range to {
		to[i] = clients.New()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		broke.SendMessageQuickly(message.NewLogicMessage("l", to, nil, []byte("x")))
	}
	waitFor(b, &ep.sent, int64(b.N))
	b.StopTimer()
	cancel()
}

// BenchmarkHeadOfLine: latency of a DISPATCHED request while an unrelated
// WORKER is busy for 1ms per message. Reports p50/p99 latency.
func BenchmarkHeadOfLine(b *testing.B) {
	broke, _, cancel := newBroker(b)
	lat := make(chan time.Duration, 1)
	_ = broke.AddLogic(&fnLogic{GoBroke.NewLogicBase("slow", types.WORKER, broke), func(types.Message) { time.Sleep(time.Millisecond) }})
	_ = broke.AddLogic(&fnLogic{GoBroke.NewLogicBase("ping", types.DISPATCHED, broke), func(m types.Message) {
		lat <- time.Since(m.Metadata["t"].(time.Time))
	}})
	go broke.Start()
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Go(func() { // keep the WORKER busy
		for {
			select {
			case <-stop:
				return
			default:
				broke.SendMessage(message.NewLogicMessage("load", nil, []types.LogicName{"slow"}, nil))
			}
		}
	})
	time.Sleep(20 * time.Millisecond)
	samples := make([]time.Duration, 0, b.N)
	b.ResetTimer()
	for range b.N {
		broke.SendMessage(message.NewLogicMessage("t", nil, []types.LogicName{"ping"}, nil,
			message.WithMetadata(map[string]any{"t": time.Now()})))
		samples = append(samples, <-lat)
	}
	b.StopTimer()
	close(stop)
	slices.Sort(samples)
	b.ReportMetric(float64(samples[len(samples)/2].Microseconds()), "p50-µs")
	b.ReportMetric(float64(samples[len(samples)*99/100].Microseconds()), "p99-µs")
	// Old router: the busy WORKER blocks the loop; drain before cancel so no send races a closed queue.
	go func() {
		for range lat {
		}
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	wg.Wait()
}
