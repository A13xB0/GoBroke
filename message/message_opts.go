package message

import (
	"github.com/google/uuid"
)

type messageOptsFunc func(*messageOpts)

type messageOpts struct {
	metadata map[string]any
	uuid     string
}

func defaultOpts() messageOpts {
	return messageOpts{
		metadata: nil,
		uuid:     uuid.New().String(),
	}
}

func WithMetadata(metadata map[string]any) messageOptsFunc {
	return func(opts *messageOpts) {
		opts.metadata = metadata
	}
}

func WithUUID(uuid string) messageOptsFunc {
	return func(opts *messageOpts) {
		opts.uuid = uuid
	}
}
