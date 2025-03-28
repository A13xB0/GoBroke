// Package GoBroke provides configuration options for the GoBroke message broker system.
package GoBroke

import "context"

// brokeOptsFunc is a function type that modifies brokeOpts.
// It's used to implement the functional options pattern for broker configuration.
type brokeOptsFunc func(*brokeOpts)

// brokeOpts holds configuration options for the GoBroke broker.
type brokeOpts struct {
	channelSize                        int             // Size of message channels for queuing
	ctx                                context.Context // Context for cancellation and value propagation
	experimentalOverrideDirectMessages bool
}

// defaultOpts returns a brokeOpts with default values.
// By default, it sets a channel size of 100 and uses context.Background().
func defaultOpts() brokeOpts {
	return brokeOpts{
		channelSize:                        100,
		ctx:                                context.Background(),
		experimentalOverrideDirectMessages: false,
	}
}

// WithChannelSize returns a brokeOptsFunc that sets the message channel buffer size.
// This affects how many messages can be queued before blocking occurs.
func WithChannelSize(size int) brokeOptsFunc {
	return func(opts *brokeOpts) {
		opts.channelSize = size
	}
}

// WithContext returns a brokeOptsFunc that sets a custom context for the broker.
// The context can be used to control the broker's lifecycle and pass values.
func WithContext(ctx context.Context) brokeOptsFunc {
	return func(opts *brokeOpts) {
		opts.ctx = ctx
	}
}
