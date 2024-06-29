package clients

import "time"

type Client struct {
	uuid        string
	metadata    map[string]any
	lastMessage time.Time
}

func New(opts ...clientOptsFunc) *Client {
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}
	return &Client{
		uuid: o.uuid,
	}
}

func (c *Client) GetUUID() string {
	return c.uuid
}

func (c *Client) AddMetadata(name string, value any) {
	c.metadata[name] = value
}

func (c *Client) GetMetadata(name string) any {
	return c.metadata[name]
}

func (c *Client) GetLastMessage() time.Time {
	return c.lastMessage
}

func (c *Client) SetLastMessageNow() {
	c.lastMessage = time.Now()
}
