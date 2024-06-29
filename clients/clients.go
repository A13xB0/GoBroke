package clients

type Client struct {
	uuid     string
	Metadata map[string]any
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
