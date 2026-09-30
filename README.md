# GoBroke

GoBroke is a lightweight internal message routing system designed for modular logic processing in Go applications. It provides a clean architecture for handling messages between different components (clients and logic modules) within your project.  
  
**It must be noted that this project may not be for you... GoBroke routes messages inside one process. For messaging between processes, look at NATS, Kafka or Redis.**

## Overview

GoBroke acts as a message router that:
- Routes messages between clients and logic modules
- Supports different types of logic processing (Dispatched, Worker, Passive)
- Allows custom endpoint implementations (HTTP, UDP, TCP, gRPC, etc.)
- Provides clean separation between message handling and business logic

## Architecture

### Core Components

1. **Broke**: The main router that manages message flow between clients and logic modules
2. **Endpoint**: Interface for implementing custom network protocols
3. **Client**: Represents connected clients and manages their state
4. **Logic**: Interface for implementing business logic modules
5. **Message**: Structure for passing data between components
6. **LogicBase**: Base implementation providing common logic functionality
7. **Lanes**: Per-WORKER (or per-queue, per-key) sequential runners

### Message Flow

```
[Client] <-> [Endpoint] <-> [Broke] <-> [Logic Modules]
```

Messages can flow:
- From clients to logic modules
- From logic modules to specific clients
- From logic modules to all clients (broadcast)

## Logic Implementation

All logic modules in GoBroke extend the `LogicBase` struct, which provides common functionality:

```go
type LogicBase struct {
    // Ctx is cancelled when the broker shuts down.
    Ctx context.Context
    *Broke // SendMessage, SendMessageQuickly, Deliver, Tick, GetClient, ...
    // name, type and scheduling options are unexported
}
```

To create a new logic module, embed `LogicBase` and initialize it using `NewLogicBase`:

```go
type customLogic struct {
    GoBroke.LogicBase
    // Additional fields specific to your logic
}

func CreateCustomLogic(broke *GoBroke.Broke) types.Logic {
    logic := customLogic{
        LogicBase: GoBroke.NewLogicBase("customlogic", types.DISPATCHED, broke),
        // Initialize additional fields
    }
    return &logic
}
```

## Logic Types

Every logic declares how it runs when it is created, with
`GoBroke.NewLogicBase(name, type, broke, options...)`. The type tells a reader
how that code is scheduled; the broker guarantees it.

| Type | Receives messages | Runs | Use for |
|---|---|---|---|
| `DISPATCHED` | yes | each message on its own goroutine | independent requests: logins, lookups, replies |
| `WORKER` | yes | one message at a time, in arrival order, on its own lane | state that must change in order: movement, a market, a guild |
| `PASSIVE` | no | only its optional `Run` loop | clocks, simulation ticks, housekeeping |

There is no central routing goroutine. A busy logic of one type never delays
another logic.

### DISPATCHED

```go
type ping struct{ GoBroke.LogicBase }

func CreatePing(b *GoBroke.Broke) types.Logic {
    return &ping{GoBroke.NewLogicBase("session.Ping", types.DISPATCHED, b)}
}

func (p *ping) RunLogic(m types.Message) error {
    return p.Deliver(message.NewSimpleLogicMessage(p.Name(), m.FromClient, "", pong))
}
```

`MaxConcurrent(n)` caps how many run at once for client traffic; further
client messages wait at the edge (see Backpressure).

### WORKER

A WORKER gets its own lane, so `RunLogic` runs one message at a time in the
order they arrived, without holding up anything else. It never needs to start
goroutines of its own.

Two options set what a WORKER is sequential *with*:

```go
// Several logics that change the same state share one lane:
// every market packet runs one at a time, in order.
GoBroke.NewLogicBase("market.buy", types.WORKER, b, GoBroke.InQueue("market"))
GoBroke.NewLogicBase("market.consign", types.WORKER, b, GoBroke.InQueue("market"))

// One queue split into lanes by key: each map runs in order, maps run in parallel.
byMap := func(m types.Message) string { return mapOf(m.FromClient) }
GoBroke.NewLogicBase("walk", types.WORKER, b, GoBroke.InQueue("world"), GoBroke.KeyedBy(byMap, 64))
GoBroke.NewLogicBase("attack", types.WORKER, b, GoBroke.InQueue("world"), GoBroke.KeyedBy(byMap, 64))
```

Keys that hash to the same lane share it, which costs parallelism but never
ordering. Every logic in a keyed queue must use the same shard count.

### PASSIVE and background loops

A PASSIVE logic receives no messages. Any logic may implement `Runner`; the
broker starts `Run` in `Start` and cancels its context on shutdown.

```go
type auctionClock struct{ GoBroke.LogicBase }

func (c *auctionClock) RunLogic(types.Message) error { return nil }

func (c *auctionClock) Run(ctx context.Context) error {
    t := time.NewTicker(10 * time.Minute)
    defer t.Stop()
    for {
        select {
        case <-ctx.Done():
            return nil
        case <-t.C:
            c.Tick("market.expire") // runs in the market lane, in order with buys
        }
    }
}
```

A clock owns time, not state: it hands time-driven work to the WORKER that owns
the state with `Tick(name)` (every lane) or `TickFor(name, key)` (one key). A
tick is skipped if one is already waiting, so a slow lane never builds a
backlog. The WORKER recognises a tick with `GoBroke.TickKey(m)`.

## Getting Started

1. Create a broker. If the endpoint implements `GoBroke.Binder`, `New` calls
   `Bind(broker)` so the endpoint has the broker without being patched later:

```go
gb, err := GoBroke.New(yourEndpoint,
    GoBroke.WithContext(ctx),
    GoBroke.WithLogger(slog.Default()),
)
if err != nil {
    return err
}
```

2. Add logic:

```go
_ = gb.AddLogic(broadcaster.CreateDispatched(gb))
_ = gb.AddLogic(inactivitymonitor.Create(gb, 15*time.Minute))
```

3. Start. `Start` blocks until the context ends; when it returns every lane and
   `Run` loop has stopped:

```go
gb.Start()
```

## Custom Endpoints

An endpoint implements `endpoint.Endpoint`:

- `Sender(ch)`: the broker's outbound channel. Read it until the broker's
  context ends; the broker never closes it.
- `Receiver(ch)`: a legacy inbound channel. Prefer calling `Receive`.
- `Disconnect(client)`: close the client's connection.
- `Start(ctx)`: the broker runs it on its own goroutine; it may block or return.

Feed client messages in with `broker.Receive(ctx, msg)` from each
connection's own goroutine. It applies receive middleware and routes the
message, and it returns `ErrorMessageRejected` if middleware rejected it.

### Backpressure

Pushing to a WORKER lane never blocks, so a logic may message its own lane.
Limits apply only to client traffic: when a lane holds `WithLaneLimit` messages
(default 1024), `Receive` waits for space. Only the calling connection waits;
with a transport that reads frames on demand, that also slows the client down.

### Shutdown

Cancel the broker's context. `SendMessage` becomes a no-op, `Deliver` returns
`ErrorBrokerNotRunning`, every `LogicBase.Ctx` is cancelled, lanes stop
(messages still queued are dropped and counted in a debug log) and `Start`
returns once lanes and `Run` loops have exited. No channel is closed, so late
senders never panic.

## Message Structure

Messages in GoBroke contain:
- Target clients (`ToClient`)
- Target logic modules (`ToLogic`)
- Source client (`FromClient`)
- Source logic module (`FromLogic`)
- Raw message data (`MessageRaw`)
- Metadata for additional context (`Metadata`)
- Unique identifier (`UUID`)
- Message state (`State`)
- Tags for middleware processing (`Tags`)

### Message State and Control

Messages can be in one of two states:
- `ACCEPTED` (default): Message continues through the processing pipeline
- `REJECTED`: Message is dropped from the processing pipeline

Control methods:
```go
// Accept the message for further processing
message.Accept()

// Reject the message to prevent further processing
message.Reject()
```

### Message Tags

Tags provide a way to attach and retrieve arbitrary data during message processing:
```go
// Add a tag to the message
message.AddTag("priority", "high")

// Retrieve a tag value
value, err := message.GetTag("priority", nil)
```

For tags you use in several places, declare a typed key once. `Get` reports
a missing tag or a wrong type with `ok == false` instead of panicking:

```go
var UserID = types.NewKey[string]("userid")

UserID.Set(&msg, "42")          // in middleware
id, ok := UserID.Get(msg)       // in logic
```

### Building replies

```go
// to the sender only; FromLogic tells the client what kind of message it is
p.Deliver(message.Reply(p.Name(), msg.FromClient, raw))

// the same message to many clients
p.Deliver(message.Notify("group.Disbanded", raw, members...))
```

## Middleware

GoBroke supports middleware functions for both receiving and sending messages. Middleware can modify messages, add tags, or control message flow through accept/reject states.

### Adding Middleware

```go
// Middleware function type
type middlewareFunc func(types.Message) types.Message

// Add receive middleware (executed when messages are received)
gb.AttachReceiveMiddleware(func(msg types.Message) types.Message {
    // Process incoming message
    return msg
})

// Add send middleware (executed before messages are sent)
gb.AttachSendMiddleware(func(msg types.Message) types.Message {
    // Process outgoing message
    return msg
})
```

Receive middleware runs for every routed message, from clients and from
logic. Send middleware runs in `SendMessage`, `SendMessageQuickly` and
`Deliver`. A middleware that calls `msg.Reject()` stops the chain and the
message is dropped; `Receive` and `Deliver` then return `ErrorMessageRejected`.
Middleware can be attached at any time.

Example middleware for message filtering:
```go
gb.AttachReceiveMiddleware(func(msg types.Message) types.Message {
    // Reject messages larger than 1MB
    if len(msg.MessageRaw) > 1024*1024 {
        msg.Reject()
    }
    return msg
})
```

## Choosing a type

Ask three questions for each handler:

1. **Does it change state that another handler also changes?** If not
   (stateless, read-only, or the database enforces it), it is `DISPATCHED`.
2. **If it does, it is a `WORKER` in the queue that owns that state.** Key the
   queue by the smallest thing that must stay consistent: a map, a guild, a
   mailbox.
3. **Is it triggered by time rather than a message?** Then it is `PASSIVE` with
   `Run`. If it changes state, it `Tick`s the owning queue rather than touching
   the state itself.

A handler never reaches into another queue's state. When an action spans two
(say, taking gold from a player and putting it in a guild bank) it does its
half and sends a message to the queue that owns the other half.

## Releasing (module tags)

This module is consumed as `github.com/A13xB0/GoBroke`. After merging changes to `main`:

```bash
git tag -a v0.X.Y -m "v0.X.Y: summary"
git push origin main
git push origin v0.X.Y
```

Downstream repos should bump `require github.com/A13xB0/GoBroke v0.X.Y` and remove any `replace ... => ../GoBroke` once the tag is visible on GitHub.

## License

This project is licensed under the terms specified in the LICENSE file.

## Note

This is primarily a personal project focused on clean architecture and modular design in Go. While it's functional and can be used in other projects, it's primarily meant as a learning tool and reference implementation.
