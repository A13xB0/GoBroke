// Package GoBroke provides a flexible message broker implementation for handling
// client-to-client and client-to-logic communication patterns. It supports
// different types of message routing, client management, and custom logic handlers.
//
// Logic types keep their meaning:
//   - DISPATCHED logic runs each message on its own goroutine.
//   - WORKER logic runs one message at a time, in order, on its own lane. Use
//     InQueue to share a lane between logics and KeyedBy to split it per key.
//   - PASSIVE logic receives no messages. Any logic may implement Runner to
//     get a background loop that the broker starts and stops.
//
// There is no central routing goroutine: SendMessage and Receive route on the
// caller's goroutine, so one busy logic never delays another.
package GoBroke

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/A13xB0/GoBroke/clients"
	"github.com/A13xB0/GoBroke/endpoint"
	brokeerrors "github.com/A13xB0/GoBroke/errors"
	"github.com/A13xB0/GoBroke/types"
)

type middlewareFunc func(types.Message) types.Message

// Binder is implemented by endpoints that need the broker, for example to
// call RegisterClient or Receive. New calls Bind before returning, so the
// endpoint never has to be patched with the broker afterwards.
type Binder interface {
	Bind(*Broke)
}

// logicEntry is a registered logic plus how the broker runs it.
type logicEntry struct {
	logic        types.Logic
	queue        *queue        // WORKER only
	sem          chan struct{} // DISPATCHED with MaxConcurrent only
	dispatchTick atomic.Bool   // DISPATCHED: a tick is in flight
	runnerOn     atomic.Bool   // Runner already started
}

// Broke represents a message broker instance that manages client connections,
// message routing, and custom logic handlers.
type Broke struct {
	endpoint endpoint.Endpoint

	logicMu sync.Mutex                                      // serialises changes to logic and queues
	logic   atomic.Pointer[map[types.LogicName]*logicEntry] // read lock-free when routing
	queues  map[string]*queue

	clients      map[string]*clients.Client
	clientsMutex sync.RWMutex

	sendQueue    chan types.Message // outbound, drained by the endpoint
	receiveQueue chan types.Message // legacy inbound channel for endpoints that don't call Receive

	recvMiddleware atomic.Pointer[[]middlewareFunc]
	sendMiddleware atomic.Pointer[[]middlewareFunc]

	ctx          context.Context
	done         <-chan struct{} // ctx.Done(), cached: ctx.Err() takes a lock
	log          *slog.Logger
	laneLimit    int
	onLogicPanic func(types.LogicName, types.Message, any, string)

	started atomic.Bool
	wg      sync.WaitGroup // lanes, runners and the legacy dispatcher
}

// New creates a new GoBroke instance with the specified endpoint and optional configuration.
// It returns an error if the endpoint is nil or if there are issues setting up message queues.
// If the endpoint implements Binder, Bind is called before New returns.
func New(endpoint endpoint.Endpoint, opts ...Option) (*Broke, error) {
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}
	if endpoint == nil {
		return nil, errors.Join(brokeerrors.ErrorCouldNotCreateServer, brokeerrors.ErrorNoEndpointProvided)
	}

	gb := &Broke{
		endpoint:     endpoint,
		queues:       make(map[string]*queue),
		clients:      make(map[string]*clients.Client),
		receiveQueue: make(chan types.Message, o.channelSize),
		sendQueue:    make(chan types.Message, o.channelSize),
		ctx:          o.ctx,
		done:         o.ctx.Done(),
		log:          o.logger,
		laneLimit:    o.laneLimit,
		onLogicPanic: o.OnLogicPanic,
	}
	empty := make(map[types.LogicName]*logicEntry)
	gb.logic.Store(&empty)
	gb.recvMiddleware.Store(new([]middlewareFunc))
	gb.sendMiddleware.Store(new([]middlewareFunc))

	if err := endpoint.Sender(gb.sendQueue); err != nil {
		return nil, errors.Join(brokeerrors.ErrorCouldNotCreateServer, err)
	}
	if err := endpoint.Receiver(gb.receiveQueue); err != nil {
		return nil, errors.Join(brokeerrors.ErrorCouldNotCreateServer, err)
	}
	if b, ok := endpoint.(Binder); ok {
		b.Bind(gb)
	}
	return gb, nil
}

// AddLogic adds a new logic handler to the GoBroke instance.
// It returns an error if a logic handler with the same name already exists, or
// if it joins a shared queue with a different shard count.
func (broke *Broke) AddLogic(logic types.Logic) error {
	broke.logicMu.Lock()
	defer broke.logicMu.Unlock()

	current := *broke.logic.Load()
	if _, ok := current[logic.Name()]; ok {
		return brokeerrors.ErrorLogicAlreadyExists
	}
	var opts logicOpts
	if s, ok := logic.(scheduled); ok {
		opts = s.scheduling()
	}
	e := &logicEntry{logic: logic}

	switch logic.Type() {
	case types.WORKER:
		q, err := broke.queueFor(logic.Name(), opts)
		if err != nil {
			return err
		}
		e.queue = q
	case types.DISPATCHED:
		if opts.maxConcurrent > 0 {
			e.sem = make(chan struct{}, opts.maxConcurrent)
		}
	}

	next := maps.Clone(current)
	next[logic.Name()] = e
	broke.logic.Store(&next)

	if broke.started.Load() {
		broke.startEntry(e)
	}
	return nil
}

// queueFor returns the queue for a WORKER, creating it if needed. Caller holds logicMu.
func (broke *Broke) queueFor(name types.LogicName, opts logicOpts) (*queue, error) {
	qname := opts.queue
	if qname == "" {
		qname = "logic:" + string(name) // private queue
	}
	if q, ok := broke.queues[qname]; ok {
		if opts.key != nil && opts.shards != q.shards {
			return nil, fmt.Errorf("%w: %q has %d, %q asked for %d", brokeerrors.ErrorQueueConflict, qname, q.shards, name, opts.shards)
		}
		return q, nil
	}
	q := newQueue(qname, opts.key, opts.shards, broke.laneLimit)
	broke.queues[qname] = q
	if broke.started.Load() {
		broke.startQueue(q)
	}
	return q, nil
}

// RemoveLogic removes a logic handler from the GoBroke instance by its name.
// It returns nil even if the logic handler doesn't exist. Messages already
// queued for it still run.
func (broke *Broke) RemoveLogic(name types.LogicName) error {
	broke.logicMu.Lock()
	defer broke.logicMu.Unlock()
	current := *broke.logic.Load()
	if _, ok := current[name]; !ok {
		return nil
	}
	next := maps.Clone(current)
	delete(next, name)
	broke.logic.Store(&next)
	return nil
}

func (broke *Broke) lookup(name types.LogicName) (*logicEntry, bool) {
	e, ok := (*broke.logic.Load())[name]
	return e, ok
}

// RegisterClient registers a new client in the GoBroke instance.
// This method should be called from the endpoint implementation.
// It returns an error if the client is already registered.
func (broke *Broke) RegisterClient(client *clients.Client) error {
	broke.clientsMutex.Lock()
	defer broke.clientsMutex.Unlock()
	if _, ok := broke.clients[client.GetUUID()]; ok {
		return brokeerrors.ErrorClientAlreadyExists
	}
	broke.clients[client.GetUUID()] = client
	client.SetLastMessageNow()
	return nil
}

// RemoveClient removes a client from the GoBroke instance and disconnects them
// from the endpoint.
func (broke *Broke) RemoveClient(client *clients.Client) error {
	clientID := client.GetUUID()

	broke.clientsMutex.RLock()
	_, ok := broke.clients[clientID]
	broke.clientsMutex.RUnlock()
	if !ok {
		return brokeerrors.ErrorClientDoesNotExist
	}

	if err := broke.endpoint.Disconnect(client); err != nil {
		return errors.Join(brokeerrors.ErrorClientCouldNotBeDisconnected, err)
	}

	broke.clientsMutex.Lock()
	delete(broke.clients, clientID)
	broke.clientsMutex.Unlock()
	return nil
}

// GetClient retrieves a client by their UUID.
// It returns the client instance and nil if found, or nil and an error if not found.
//
// The localOnly parameter is ignored (Redis support was removed); it is kept for source compatibility.
func (broke *Broke) GetClient(uuid string, localOnly ...bool) (*clients.Client, error) {
	_ = localOnly
	broke.clientsMutex.RLock()
	defer broke.clientsMutex.RUnlock()
	if client, ok := broke.clients[uuid]; ok {
		return client, nil
	}
	return nil, brokeerrors.ErrorClientDoesNotExist
}

// GetAllClients returns a slice containing all currently connected clients.
//
// The localOnly parameter is ignored (Redis support was removed); it is kept for source compatibility.
func (broke *Broke) GetAllClients(localOnly ...bool) []*clients.Client {
	_ = localOnly
	broke.clientsMutex.RLock()
	defer broke.clientsMutex.RUnlock()
	cl := make([]*clients.Client, 0, len(broke.clients))
	for _, value := range broke.clients {
		cl = append(cl, value)
	}
	return cl
}

// SendMessage routes a message to its logic handlers and clients.
// It is for messages produced inside the server (logics, clocks, admin
// tools) and never waits for lane space, so a logic can safely message itself.
// Client traffic should enter through Receive instead.
// If the message is from a client, their last message timestamp is updated.
func (broke *Broke) SendMessage(message types.Message) {
	message, ok := broke.applySendMiddleware(message)
	if !ok {
		return
	}
	if message.FromClient != nil {
		message.FromClient.SetLastMessageNow()
	}
	_ = broke.route(broke.ctx, message, false)
}

// Receive is the entry point for messages from clients. Endpoints call it on
// the connection's own goroutine. It applies receive middleware and routes the
// message; when a WORKER lane (or a MaxConcurrent DISPATCHED logic) is full it
// waits for space, which slows only this connection. It returns
// ErrorMessageRejected when middleware rejects the message and ctx's cause if
// ctx ends while waiting.
func (broke *Broke) Receive(ctx context.Context, message types.Message) error {
	if message.FromClient != nil {
		message.FromClient.SetLastMessageNow()
	}
	return broke.route(ctx, message, true)
}

// SendMessageQuickly sends a message straight to its client recipients,
// bypassing logic handlers. It is the usual way for a logic to reply.
func (broke *Broke) SendMessageQuickly(message types.Message) {
	message, ok := broke.applySendMiddleware(message)
	if !ok {
		return
	}
	message.SentQuickly = true
	broke.deliver(message)
}

// Deliver is SendMessageQuickly that reports whether the message was handed to
// the endpoint: it returns ErrorMessageRejected if send middleware rejected it
// and ErrorBrokerNotRunning if the broker has shut down.
func (broke *Broke) Deliver(message types.Message) error {
	message, ok := broke.applySendMiddleware(message)
	if !ok {
		return brokeerrors.ErrorMessageRejected
	}
	message.SentQuickly = true
	if !broke.deliver(message) {
		return brokeerrors.ErrorBrokerNotRunning
	}
	return nil
}

// route runs receive middleware and hands the message to clients and logics.
// edge is true for client traffic, which may wait for lane space.
func (broke *Broke) route(ctx context.Context, msg types.Message, edge bool) error {
	if msg.SentQuickly {
		broke.deliver(msg)
		return nil
	}
	msg, ok := broke.applyRecvMiddleware(msg)
	if !ok {
		return brokeerrors.ErrorMessageRejected
	}
	if len(msg.ToClient) != 0 {
		broke.deliver(msg)
	}
	for _, name := range msg.ToLogic {
		e, found := broke.lookup(name)
		if !found {
			continue
		}
		if err := broke.handOff(ctx, e, msg, edge); err != nil {
			return err
		}
	}
	return nil
}

func (broke *Broke) handOff(ctx context.Context, e *logicEntry, msg types.Message, edge bool) error {
	switch e.logic.Type() {
	case types.WORKER:
		l := e.queue.laneFor(msg)
		if edge {
			if err := l.admit(ctx); err != nil {
				return err
			}
		}
		l.push(job{entry: e, msg: msg})
	case types.DISPATCHED:
		if e.sem == nil {
			go broke.runLogic(e, msg)
			return nil
		}
		if edge {
			select {
			case e.sem <- struct{}{}:
			case <-ctx.Done():
				return context.Cause(ctx)
			}
			broke.dispatchHeld(e, msg)
			return nil
		}
		select {
		case e.sem <- struct{}{}:
			broke.dispatchHeld(e, msg)
		default:
			go broke.runLogic(e, msg) // internal traffic is never held back
		}
	default:
		// PASSIVE logic does not receive messages.
	}
	return nil
}

// dispatch runs a DISPATCHED logic on a new goroutine. clear, if set, is reset
// when the run starts (used to coalesce ticks).
func (broke *Broke) dispatch(e *logicEntry, msg types.Message, clear *atomic.Bool) {
	go func() {
		if clear != nil {
			clear.Store(false)
		}
		broke.runLogic(e, msg)
	}()
}

// dispatchHeld runs a DISPATCHED logic whose MaxConcurrent slot is already taken.
func (broke *Broke) dispatchHeld(e *logicEntry, msg types.Message) {
	go func() {
		defer func() { <-e.sem }()
		broke.runLogic(e, msg)
	}()
}

// deliver hands a message to the endpoint. It reports false if the broker
// shut down first; the outbound channel is never closed, so this never panics.
func (broke *Broke) deliver(msg types.Message) bool {
	select {
	case <-broke.done:
		return false
	default:
	}
	select { // fast path: room in the channel
	case broke.sendQueue <- msg:
		return true
	default:
	}
	select { // endpoint is behind: wait, but not past shutdown
	case broke.sendQueue <- msg:
		return true
	case <-broke.done:
		return false
	}
}

func (broke *Broke) applyRecvMiddleware(msg types.Message) (types.Message, bool) {
	for _, fn := range *broke.recvMiddleware.Load() {
		msg = fn(msg)
		if msg.State == types.REJECTED {
			return msg, false
		}
	}
	return msg, msg.State != types.REJECTED
}

func (broke *Broke) applySendMiddleware(msg types.Message) (types.Message, bool) {
	for _, fn := range *broke.sendMiddleware.Load() {
		msg = fn(msg)
		if msg.State == types.REJECTED {
			return msg, false
		}
	}
	return msg, msg.State != types.REJECTED
}

// AttachReceiveMiddleware adds a middleware function to the receive pipeline,
// which runs for every routed message (client and internal). Middleware runs
// in the order attached; a middleware that calls msg.Reject() stops the
// pipeline and the message is dropped. Safe to call at any time.
func (broke *Broke) AttachReceiveMiddleware(mFunc middlewareFunc) {
	appendMiddleware(&broke.recvMiddleware, mFunc)
}

// AttachSendMiddleware adds a middleware function to the send pipeline, which
// runs in SendMessage, SendMessageQuickly and Deliver. A middleware that calls
// msg.Reject() drops the message. Safe to call at any time.
func (broke *Broke) AttachSendMiddleware(mFunc middlewareFunc) {
	appendMiddleware(&broke.sendMiddleware, mFunc)
}

func appendMiddleware(p *atomic.Pointer[[]middlewareFunc], fn middlewareFunc) {
	for {
		old := p.Load()
		next := append(append(make([]middlewareFunc, 0, len(*old)+1), *old...), fn)
		if p.CompareAndSwap(old, &next) {
			return
		}
	}
}

// GetEndpoint returns the endpoint used by this broker.
// This can be useful for endpoint-specific operations.
func (broke *Broke) GetEndpoint() endpoint.Endpoint {
	return broke.endpoint
}

// Start starts the endpoint, the WORKER lanes, every Runner and the legacy
// inbound channel, then blocks until the broker's context ends. On return all
// lanes and runners have stopped. The outbound channel is never closed;
// endpoints should stop when the context ends.
func (broke *Broke) Start() {
	if !broke.started.CompareAndSwap(false, true) {
		<-broke.ctx.Done()
		return
	}
	broke.logicMu.Lock()
	for _, q := range broke.queues {
		broke.startQueue(q)
	}
	for _, e := range *broke.logic.Load() {
		broke.startEntry(e)
	}
	broke.logicMu.Unlock()

	broke.wg.Go(broke.drainLegacyReceiver)
	go broke.endpoint.Start(broke.ctx) // may block (StubEndpoint) or return at once

	<-broke.ctx.Done()
	broke.wg.Wait()
}

func (broke *Broke) startQueue(q *queue) {
	for i, l := range q.lanes {
		broke.wg.Go(func() {
			if dropped := l.run(broke.ctx, broke.runJob); dropped > 0 {
				broke.log.Debug("gobroke: lane stopped with queued messages", "queue", q.name, "lane", i, "dropped", dropped)
			}
		})
	}
}

func (broke *Broke) startEntry(e *logicEntry) {
	r, ok := e.logic.(Runner)
	if !ok || !e.runnerOn.CompareAndSwap(false, true) {
		return
	}
	broke.wg.Go(func() { broke.runRunner(e.logic.Name(), r) })
}

// drainLegacyReceiver routes messages that endpoints write to the Receiver
// channel. Prefer calling Receive from each connection's goroutine: this
// single goroutine is shared by every connection that uses the channel.
func (broke *Broke) drainLegacyReceiver() {
	for {
		select {
		case <-broke.ctx.Done():
			return
		case msg := <-broke.receiveQueue:
			if err := broke.Receive(broke.ctx, msg); err != nil && !errors.Is(err, brokeerrors.ErrorMessageRejected) {
				broke.log.Debug("gobroke: legacy receive failed", "err", err)
			}
		}
	}
}

func (broke *Broke) runJob(j job) {
	broke.runLogic(j.entry, j.msg)
}

// runLogic runs RunLogic, recovering from panics and logging errors. It is
// the entry point of every DISPATCHED goroutine, so its frame is kept small:
// the rare paths live in separate functions and a new goroutine's initial
// stack is enough (growing it cost ~30% of DISPATCHED throughput).
func (broke *Broke) runLogic(e *logicEntry, msg types.Message) {
	defer func() {
		if r := recover(); r != nil {
			broke.reportPanic(e.logic.Name(), msg, r)
		}
	}()
	if err := e.logic.RunLogic(msg); err != nil {
		broke.logLogicError(e.logic.Name(), err)
	}
}

//go:noinline
func (broke *Broke) logLogicError(name types.LogicName, err error) {
	broke.log.Warn("gobroke: logic returned an error", "logic", string(name), "err", err)
}

func (broke *Broke) runRunner(name types.LogicName, r Runner) {
	defer func() {
		if rec := recover(); rec != nil {
			broke.reportPanic(name, types.Message{}, rec)
		}
	}()
	if err := r.Run(broke.ctx); err != nil && !errors.Is(err, context.Canceled) {
		broke.log.Warn("gobroke: runner stopped with an error", "logic", string(name), "err", err)
	}
}

// reportPanic reports a recovered panic through OnLogicPanic, or stderr.
//
//go:noinline
func (broke *Broke) reportPanic(name types.LogicName, msg types.Message, r any) {
	stack := string(debug.Stack())
	if broke.onLogicPanic != nil {
		broke.onLogicPanic(name, msg, r, stack)
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "logic panic logic=%s recovered=%v\n%s\n", name, r, stack)
}
