package peer

import (
	"github.com/btcsuite/btclog"
	"github.com/cryptomeow/lnd/build"
)

// peerLog is a logger that is initialized with the btclog.Disabled logger.
var peerLog btclog.Logger

// The default amount of logging is none.
func init() {
	UseLogger(build.NewSubLogger("PEER", nil))
}

// DisableLog disables all logging output.
func DisableLog() {
	UseLogger(btclog.Disabled)
}

// UseLogger uses a specified Logger to output package logging info.
func UseLogger(logger btclog.Logger) {
	peerLog = logger
}

// logClosure is used to provide a closure over expensive logging operations
// so they aren't performed when the logging level doesn't warrant it.
type logClosure func() string

// String invokes the underlying function and returns the result.
func (c logClosure) String() string {
	return c()
}

// newLogClosure returns a new closure over a function that returns a string
// which itself provides a Stringer interface so that it can be used with the
// logging system.
func newLogClosure(c func() string) logClosure {
	return logClosure(c)
}
