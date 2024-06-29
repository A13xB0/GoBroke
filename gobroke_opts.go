package GoBroke

import "context"

type brokeOptsFunc func(*brokeOpts)

type brokeOpts struct {
	channelSize int
	ctx         context.Context
}

func defaultOpts() brokeOpts {
	return brokeOpts{
		channelSize: 100,
		ctx:         context.Background(),
	}
}

func WithChannelSize(size int) brokeOptsFunc {
	return func(opts *brokeOpts) {
		opts.channelSize = size
	}
}

func WithContext(ctx context.Context) brokeOptsFunc {
	return func(opts *brokeOpts) {
		opts.ctx = ctx
	}
}
