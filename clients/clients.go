// Package clients provides client management functionality for the GoBroke system.
// It handles client creation, metadata management, and tracking of client activity.
package clients

import "time"

// Client represents a connected client in the GoBroke system.
// Each client has a unique identifier, optional metadata, and activity tracking.
type Client struct {
	uuid        string         // Unique identifier for the client
	metadata    map[string]any // Optional key-value pairs associated with the client
	lastMessage time.Time      // Timestamp of the client's most recent message
}

// New creates a new Client instance with the provided options.
// Options can include custom UUID and metadata configurations.
func New(opts ...clientOptsFunc) *Client {
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
	c.metadata[name] = value
}

// GetMetadata retrieves the value associated with the given metadata key.
// Returns nil if the key doesn't exist.
func (c *Client) GetMetadata(name string) any {
	return c.metadata[name]
}

// GetLastMessage returns the timestamp of the client's most recent message.
func (c *Client) GetLastMessage() time.Time {
	return c.lastMessage
}

// SetLastMessageNow updates the client's last message timestamp to the current time.
// This is typically called when the client sends a new message.
func (c *Client) SetLastMessageNow() {
	c.lastMessage = time.Now()
}
