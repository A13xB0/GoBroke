# GoBroke Redis Integration for High Availability

This document explains how to use the Redis integration in GoBroke to enable high availability across multiple instances.

## Overview

The Redis integration allows GoBroke instances to communicate with each other, enabling:

1. Client discovery across instances
2. Message routing between instances
3. High availability for client connections

When a client is not connected to the current GoBroke instance, the system will automatically route messages through Redis to the instance where the client is connected.

## Configuration

Redis integration is optional and can be enabled through the `WithRedis` option when creating a new GoBroke instance:

```go
// Create a Redis client
redisClient := redis.NewClient(&redis.Options{
    Addr:     "localhost:6379", // Redis server address
    Password: "",               // Redis password (if any)
    DB:       0,                // Redis database number
})

// Create a GoBroke instance with Redis integration
broker, err := GoBroke.New(
    endpoint.NewStubEndpoint(),
    GoBroke.WithContext(ctx),
    GoBroke.WithRedis(GoBroke.RedisConfig{
        Client:      redisClient,      // Use existing Redis client
        ChannelName: "gobroke:messages", // Channel for inter-instance communication
        InstanceID:  "instance-1",     // Unique identifier for this instance
    }),
)
```

### Redis Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| Enabled | Whether Redis integration is enabled | false |
| Client | Redis client instance | Required |
| ChannelName | Redis channel for inter-instance communication | "gobroke:messages" |
| InstanceID | Unique identifier for this GoBroke instance | Auto-generated timestamp |

## How It Works

### Client Registration and Removal

When a client connects to a GoBroke instance with Redis enabled:

1. The client is registered locally as usual
2. The client's UUID is also registered in Redis with the instance ID
3. This registration expires after 24 hours (to prevent stale entries)
4. The client's last message time is synchronized across all replicas every second

When a client is removed:

1. If the client is local, it is disconnected from the endpoint and removed from the local registry
2. The client is also unregistered from Redis, removing both its instance registration and last message time
3. If the client is not local but exists in Redis (on another replica), it is removed from Redis
4. This allows for managing clients across all replicas, not just the local one

### Client Discovery

The `GetAllClients` method has been enhanced to return clients from all connected instances:

1. It first collects all locally connected clients
2. If Redis is enabled, it queries Redis for clients registered on other instances
3. It creates virtual client references for remote clients and includes them in the result
4. For remote clients, it retrieves their last message time from Redis

This allows you to get a complete view of all clients across your distributed system, including when they last sent a message.

### Message Routing

When sending a message to a client:

1. GoBroke checks if the client is connected locally
2. If not, it checks if the client is registered in Redis
3. If found in Redis, the message is published to the Redis channel
4. Other GoBroke instances subscribe to this channel and process messages for their clients

### Loop Prevention

To prevent message loops, each message includes the source instance ID. When an instance receives a message from Redis, it checks if the message originated from itself and discards it if so.

## Example Usage

See the [examples/redis/main.go](examples/redis/main.go) file for a complete example of using GoBroke with Redis integration.

### Running Multiple Instances

To test high availability with multiple instances:

1. Start a Redis server
2. Run multiple instances of your GoBroke application with different instance IDs
3. Connect clients to different instances
4. Send messages between clients on different instances

### Last Message Time Synchronization

To ensure all replicas know when a client last sent a message:

1. Each replica updates Redis with the last message time for its local clients every second
2. When retrieving a client from another replica, the last message time is fetched from Redis
3. This allows replicas to know when a client was last active, even if the client is connected to a different replica
4. If a replica goes down, other replicas can still access the client's last activity timestamp

## Best Practices

1. **Instance IDs**: Use meaningful instance IDs to help with debugging and monitoring
2. **Error Handling**: Redis errors are logged but don't cause GoBroke to fail, allowing for graceful degradation
3. **Monitoring**: Consider implementing additional monitoring for Redis connectivity
4. **Scaling**: For high-volume deployments, consider using Redis Cluster or Redis Sentinel
5. **Inactivity Detection**: Use the synchronized last message times to detect inactive clients across all replicas

## Limitations

1. Client metadata is not synchronized between instances
2. Logic handlers run only on the instance that receives the message
3. Redis is a single point of failure unless you use Redis Cluster or Sentinel
