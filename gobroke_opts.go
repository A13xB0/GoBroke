// Package GoBroke provides configuration options for the GoBroke message broker system.
package GoBroke

import (
	"context"
	"log/slog"

	"github.com/A13xB0/GoBroke/types"
)

// Option configures a broker. Pass options to New.
type Option func(*brokeOpts)

// brokeOpts holds configuration options for the GoBroke broker.
type brokeOpts struct {
	channelSize int             // Size of the outbound and legacy inbound channels
	laneLimit   int             // Messages a lane holds before Receive waits
	ctx         context.Context // Context for cancellation and value propagation
	logger      *slog.Logger
	// OnLogicPanic is called after recovering from a panic in RunLogic or Run.
	OnLogicPanic func(logicName types.LogicName, msg types.Message, recovered any, stack string)
}

// defaultOpts returns a brokeOpts with default values: channel size 100,
// lane limit 1024, context.Background() and slog.Default().
func defaultOpts() brokeOpts {
	return brokeOpts{
		channelSize: 100,
		laneLimit:   defaultLaneLimit,
		ctx:         context.Background(),
		logger:      slog.Default(),
	}
}

// WithChannelSize sets the buffer size of the outbound channel handed to the
// endpoint's Sender and of the legacy inbound channel handed to Receiver.
func WithChannelSize(size int) Option {
	return func(opts *brokeOpts) {
		opts.channelSize = size
	}
}

// WithLaneLimit sets how many messages a WORKER lane may hold before a client
// message arriving through Receive waits for space. Messages sent by logics
// never wait. Default 1024.
func WithLaneLimit(n int) Option {
	return func(opts *brokeOpts) {
		opts.laneLimit = n
	}
}

// WithContext returns an Option that sets a custom context for the broker.
// Cancelling it shuts the broker down and cancels every LogicBase.Ctx.
func WithContext(ctx context.Context) Option {
	return func(opts *brokeOpts) {
		opts.ctx = ctx
	}
}

// WithLogger sets the logger for logic errors, dropped messages and shutdown.
// Default slog.Default().
func WithLogger(l *slog.Logger) Option {
	return func(opts *brokeOpts) {
		if l != nil {
			opts.logger = l
		}
	}
}

// WithOnLogicPanic sets a callback invoked when a logic handler panics. The broker always
// recovers so the process and message loop keep running; use this for structured logging.
func WithOnLogicPanic(fn func(logicName types.LogicName, msg types.Message, recovered any, stack string)) Option {
	return func(opts *brokeOpts) {
		opts.OnLogicPanic = fn
	}
}
