// Package clients provides client management functionality for the GoBroke system.
// It handles client creation, metadata management, and tracking of client activity.
package clients

import (
	"sync"
	"sync/atomic"
	"time"
)

// Client represents a connected client in the GoBroke system.
// Each client has a unique identifier, optional metadata, and activity tracking.
// All methods are safe for concurrent use.
type Client struct {
	uuid        string
	mu          sync.RWMutex
	metadata    map[string]any // allocated on first AddMetadata
	lastMessage atomic.Int64   // unix nanoseconds; 0 = never
}

// New creates a new Client instance with the provided options.
// Options can include custom UUID and metadata configurations.
func New(opts ...Option) *Client {
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}
	return &Client{
		uuid: o.uuid,
	}
}

// GetUUID returns the client's unique identifier.
func (c *Client) GetUUID() string {
	return c.uuid
}

// AddMetadata associates a key-value pair with the client.
// This can be used to store custom data related to the client.
func (c *Client) AddMetadata(name string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.metadata == nil {
		c.metadata = make(map[string]any)
	}
	c.metadata[name] = value
}

// GetMetadata retrieves the value associated with the given metadata key.
// Returns nil if the key doesn't exist.
func (c *Client) GetMetadata(name string) any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.metadata[name]
}

// GetLastMessage returns the timestamp of the client's most recent message,
// or the zero time if none has been recorded.
func (c *Client) GetLastMessage() time.Time {
	n := c.lastMessage.Load()
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}

// SetLastMessageNow updates the client's last message timestamp to the current time.
// This is typically called when the client sends a new message.
func (c *Client) SetLastMessageNow() {
	c.lastMessage.Store(time.Now().UnixNano())
}

// SetLastMessage sets the client's last message timestamp to the specified time.
func (c *Client) SetLastMessage(t time.Time) {
	if t.IsZero() {
		c.lastMessage.Store(0)
		return
	}
	c.lastMessage.Store(t.UnixNano())
}
