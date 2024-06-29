package clients

import "github.com/google/uuid"

type clientOptsFunc func(*clientOpts)

type clientOpts struct {
	uuid string
}

func defaultOpts() clientOpts {
	return clientOpts{
		uuid: uuid.New().String(),
	}
}

func WithUUID(uuid string) clientOptsFunc {
	return func(opts *clientOpts) {
		opts.uuid = uuid
	}
}
