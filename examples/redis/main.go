// Example demonstrating GoBroke with Redis high availability
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/A13xB0/GoBroke"
	"github.com/A13xB0/GoBroke/endpoint"
	"github.com/A13xB0/GoBroke/message"
	"github.com/A13xB0/GoBroke/types"
	"github.com/redis/go-redis/v9"
)

// Simple logic handler for demonstration
type echoLogic struct{}

func (e *echoLogic) RunLogic(msg types.Message) error {
	fmt.Printf("Echo Logic received message: %s\n", string(msg.MessageRaw))
	return nil
}

func (e *echoLogic) Type() types.LogicType {
	return types.WORKER
}

func (e *echoLogic) Name() types.LogicName {
	return "echo"
}

func main() {
	// Create a context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("Shutting down...")
		cancel()
	}()

	// Create a Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379", // Redis server address
		Password: "",               // Redis password (if any)
		DB:       0,                // Redis database number
	})

	// Test Redis connection
	if _, err := redisClient.Ping(ctx).Result(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Create a new GoBroke instance with Redis enabled
	// Note: Redis is optional, remove WithRedis option to disable it
	broker, err := GoBroke.New(
		endpoint.NewStubEndpoint(),
		GoBroke.WithContext(ctx),
		GoBroke.WithChannelSize(100),
		GoBroke.WithRedis(GoBroke.RedisConfig{
			Client:      redisClient,
			ChannelName: "gobroke:messages",
			InstanceID:  fmt.Sprintf("instance-%d", time.Now().UnixNano()),
		}),
	)
	if err != nil {
		log.Fatalf("Failed to create broker: %v", err)
	}

	// Add logic handler
	echo := &echoLogic{}
	if err := broker.AddLogic(echo); err != nil {
		log.Fatalf("Failed to add logic: %v", err)
	}

	// Start the broker in a goroutine
	go broker.Start()

	// Get the stub endpoint to simulate client connections
	stubEndpoint, ok := broker.GetEndpoint().(*endpoint.StubEndpoint)
	if !ok {
		log.Fatalf("Expected StubEndpoint, got %T", broker.GetEndpoint())
	}

	// Create some test clients using the endpoint
	client1 := stubEndpoint.SimulateClientConnection()
	client2 := stubEndpoint.SimulateClientConnection()

	// Register clients with the broker
	if err := broker.RegisterClient(client1); err != nil {
		log.Fatalf("Failed to register client1: %v", err)
	}
	if err := broker.RegisterClient(client2); err != nil {
		log.Fatalf("Failed to register client2: %v", err)
	}

	// Send a message from client1 to client2
	msg := message.NewSimpleClientMessage(
		client1,
		client2,
		echo.Name(),
		[]byte("Hello from client1 to client2!"),
	)
	broker.SendMessage(msg)

	// Wait for context cancellation (Ctrl+C)
	<-ctx.Done()
	fmt.Println("Exiting...")
}
