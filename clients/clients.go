package clients

import "github.com/A13xB0/GoBroke/message"

type Client struct {
	sending   chan message.Message
	receiving chan message.Message
}
