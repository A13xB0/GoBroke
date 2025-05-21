# GoBroke

GoBroke is a lightweight internal message routing system designed for modular logic processing in Go applications. It provides a clean architecture for handling messages between different components (clients and logic modules) within your project, with optional Redis integration for high availability across multiple instances.

## Overview

GoBroke acts as a message router that:
- Routes messages between clients and logic modules
- Supports different types of logic processing (Dispatched, Worker, Passive)
- Allows custom endpoint implementations (HTTP, UDP, TCP, gRPC, etc.)
- Provides clean separation between message handling and business logic
- Enables high availability through Redis integration

## Architecture

### Core Components

1. **Broke**: The main router that manages message flow between clients and logic modules
2. **Endpoint**: Interface for implementing custom network protocols
3. **Client**: Represents connected clients and manages their state
4. **Logic**: Interface for implementing business logic modules
5. **Message**: Structure for passing data between components
6. **LogicBase**: Base implementation providing common logic functionality
7. **Redis Integration**: Optional component for enabling high availability across multiple instances

### Message Flow

```
[Client] <-> [Endpoint] <-> [Broke] <-> [Logic Modules]
                             |
                        [Redis Layer]
                             |
                     [Other GoBroke Instances]
```

Messages can flow:
- From clients to logic modules
- From logic modules to specific clients
- From logic modules to all clients (broadcast)
- Between GoBroke instances via Redis (when enabled)

## Logic Implementation

All logic modules in GoBroke extend the `LogicBase` struct, which provides common functionality:

```go
type LogicBase struct {
    name      string
    logicType types.LogicType
    Ctx       context.Context
    *Broke
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

GoBroke supports three types of logic modules:

### 1. DISPATCHED Logic

- Processes messages immediately in a new goroutine
- Best for quick, non-blocking operations
- Suitable for broadcasting or simple transformations
- Example: Message broadcaster

```go
type broadcasterDispatched struct {
    GoBroke.LogicBase
}

func CreateDispatched(broke *GoBroke.Broke) types.Logic {
    worker := broadcasterDispatched{
        LogicBase: GoBroke.NewLogicBase("broadcaster", types.DISPATCHED, broke),
    }
    return &worker
}

func (w *broadcasterDispatched) RunLogic(msg types.Message) error {
    clients := w.GetAllClients()
    sMsg := types.Message{
        ToClient:   clients,
        FromLogic:  w,
        MessageRaw: msg.MessageRaw,
    }
    w.SendMessage(sMsg)
    return nil
}
```

### 2. WORKER Logic

- Processes messages in a dedicated worker goroutine
- Maintains its own message queue
- Best for sequential processing or rate-limited operations
- Example: Sequential message processor

```go
type broadcasterWorker struct {
    GoBroke.LogicBase
    receive chan types.Message
}

func CreateWorker(broke *GoBroke.Broke, ctx context.Context) types.Logic {
    worker := broadcasterWorker{
        LogicBase: GoBroke.NewLogicBase("broadcaster", types.WORKER, broke),
        receive:   make(chan types.Message),
    }
    worker.startWorker()
    return &worker
}

func (w *broadcasterWorker) startWorker() {
    for {
        select {
        case <-w.Ctx.Done():
            return
        case msg := <-w.receive:
            w.work(msg)
        }
    }
}

func (w *broadcasterWorker) RunLogic(message types.Message) error {
    w.receive <- message
    return nil
}
```

### 3. PASSIVE Logic

- Runs independently of message flow
- Never receives messages directly
- Best for background tasks or monitoring
- Example: Inactivity monitor

```go
type inactivityMonitor struct {
    GoBroke.LogicBase
    inactivityMinutes int
}

func CreateWorker(broke *GoBroke.Broke, inactivityMinutes int) types.Logic {
    worker := inactivityMonitor{
        LogicBase:         GoBroke.NewLogicBase("inactivitymonitor", types.PASSIVE, broke),
        inactivityMinutes: inactivityMinutes,
    }
    worker.startWorker()
    return &worker
}

func (w *inactivityMonitor) startWorker() {
    for {
        select {
        case <-w.Ctx.Done():
            return
        default:
            time.Sleep(10 * time.Second)
            clients := w.GetAllClients()
            for _, client := range clients {
                delta := time.Now().Sub(client.GetLastMessage())
                if delta.Minutes() > float64(w.inactivityMinutes) {
                    _ = w.RemoveClient(client)
                }
            }
        }
    }
}

func (w *inactivityMonitor) RunLogic(message types.Message) error {
    return fmt.Errorf("this logic does not support invocation")
}
```

## Getting Started

1. Create a new GoBroke instance:

```go
ctx := context.Background()

// Optional: Configure Redis for high availability
redisClient := redis.NewClient(&redis.Options{
    Addr:     "localhost:6379",
    Password: "",
    DB:       0,
})

// Create GoBroke instance with optional Redis integration
gb, err := GoBroke.New(
    yourendpoint,
    GoBroke.WithContext(ctx),
    // Optional: Enable Redis integration
    GoBroke.WithRedis(GoBroke.RedisConfig{
        Client:      redisClient,
        ChannelName: "gobroke:messages",
        InstanceID:  "instance-1",
    }),
)
if err != nil {
    panic(err)
}
```

2. Implement your logic modules:

```go
// Create and add logic modules
broadcasterLogic := broadcaster.CreateDispatched(gb)
_ = gb.AddLogic(broadcasterLogic)

inactivityMonitor := inactivitymonitor.CreateWorker(gb, 15)
_ = gb.AddLogic(inactivityMonitor)
```

3. Implement an endpoint:

```go
// Implement the endpoint.Endpoint interface
type Endpoint interface {
    Sender(chan types.Message) error
    Receiver(chan types.Message) error
    Disconnect(*clients.Client) error
}
```

4. Start the router:

```go
gb.Start()
```

## Custom Endpoints

To implement a custom endpoint:

1. Create a struct that implements the `endpoint.Endpoint` interface
2. Implement the required methods:
   - `Sender`: Handle outgoing messages
   - `Receiver`: Handle incoming messages
   - `Disconnect`: Handle client disconnection

Example WebSocket endpoint structure:
```go
type WSEndpoint struct {
    upgrader websocket.Upgrader
    clients  map[string]*websocket.Conn
}

func (e *WSEndpoint) Sender(ch chan types.Message) error {
    // Implement message sending logic
}

func (e *WSEndpoint) Receiver(ch chan types.Message) error {
    // Implement message receiving logic
}

func (e *WSEndpoint) Disconnect(client *clients.Client) error {
    // Implement client disconnection logic
}
```

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

## Redis Integration

GoBroke supports Redis integration for enabling high availability across multiple instances. When enabled, this feature allows:

1. Client discovery across instances
2. Message routing between instances
3. High availability for client connections
4. Last message time synchronization

### Configuration

Redis integration is configured through the `WithRedis` option:

```go
redisClient := redis.NewClient(&redis.Options{
    Addr:     "localhost:6379",
    Password: "",
    DB:       0,
})

broker, err := GoBroke.New(
    endpoint.NewStubEndpoint(),
    GoBroke.WithContext(ctx),
    GoBroke.WithRedis(GoBroke.RedisConfig{
        Client:      redisClient,
        ChannelName: "gobroke:messages",
        InstanceID:  "instance-1",
    }),
)
```

### How It Works

1. **Client Registration**:
   - Clients are registered both locally and in Redis
   - Registration includes instance ID and last message time
   - Registrations expire after 24 hours to prevent stale entries

2. **Message Routing**:
   - Messages are automatically routed to the correct instance
   - Uses Redis pub/sub for inter-instance communication
   - Includes loop prevention mechanisms

3. **Client Discovery**:
   - `GetAllClients` returns clients from all instances
   - Creates virtual client references for remote clients
   - Includes last message times from all instances

4. **High Availability**:
   - Clients can connect to any instance
   - Messages are automatically routed to the correct instance
   - Last message times are synchronized across instances

### Best Practices

1. Use meaningful instance IDs for debugging
2. Implement monitoring for Redis connectivity
3. Consider Redis Cluster or Sentinel for production
4. Handle Redis errors gracefully
5. Monitor client inactivity across instances

### Limitations

1. Client metadata is not synchronized between instances
2. Logic handlers run only on the instance that receives the message
3. Redis becomes a single point of failure unless using Cluster/Sentinel

For a complete example of Redis integration, see [examples/redis/main.go](examples/redis/main.go).

## Best Practices

1. **Logic Type Selection**:
   - Use DISPATCHED for simple, non-blocking operations
   - Use WORKER for sequential or rate-limited processing
   - Use PASSIVE for background tasks and monitoring

2. **Context Usage**:
   - Use the context provided by LogicBase for cancellation
   - Add timeouts where appropriate
   - Handle context cancellation in worker loops

3. **Message Processing**:
   - Keep message processing logic concise
   - Use appropriate goroutines for concurrent processing
   - Consider message ordering requirements when choosing logic types

4. **LogicBase Usage**:
   - Extend LogicBase for all logic implementations
   - Use the provided context for cancellation handling
   - Access common functionality through LogicBase methods

5. **High Availability**:
   - Use Redis integration for production deployments
   - Configure appropriate Redis timeouts
   - Monitor Redis connectivity
   - Implement proper error handling

## License

This project is licensed under the terms specified in the LICENSE file.

## Note

This is primarily a personal project focused on clean architecture and modular design in Go. While it's functional and can be used in other projects, it's primarily meant as a learning tool and reference implementation.
