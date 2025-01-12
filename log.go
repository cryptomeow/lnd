package lnd

import (
	"context"

	"github.com/btcsuite/btcd/connmgr"
	"github.com/btcsuite/btclog"
	"github.com/lightninglabs/neutrino"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/cryptomeow/lnd/autopilot"
	"github.com/cryptomeow/lnd/build"
	"github.com/cryptomeow/lnd/chainntnfs"
	"github.com/cryptomeow/lnd/chanbackup"
	"github.com/cryptomeow/lnd/chanfitness"
	"github.com/cryptomeow/lnd/channeldb"
	"github.com/cryptomeow/lnd/channelnotifier"
	"github.com/cryptomeow/lnd/contractcourt"
	"github.com/cryptomeow/lnd/discovery"
	"github.com/cryptomeow/lnd/healthcheck"
	"github.com/cryptomeow/lnd/htlcswitch"
	"github.com/cryptomeow/lnd/invoices"
	"github.com/cryptomeow/lnd/lnrpc/autopilotrpc"
	"github.com/cryptomeow/lnd/lnrpc/chainrpc"
	"github.com/cryptomeow/lnd/lnrpc/invoicesrpc"
	"github.com/cryptomeow/lnd/lnrpc/routerrpc"
	"github.com/cryptomeow/lnd/lnrpc/signrpc"
	"github.com/cryptomeow/lnd/lnrpc/verrpc"
	"github.com/cryptomeow/lnd/lnrpc/walletrpc"
	"github.com/cryptomeow/lnd/lnwallet"
	"github.com/cryptomeow/lnd/lnwallet/chancloser"
	"github.com/cryptomeow/lnd/lnwallet/chanfunding"
	"github.com/cryptomeow/lnd/monitoring"
	"github.com/cryptomeow/lnd/netann"
	"github.com/cryptomeow/lnd/peer"
	"github.com/cryptomeow/lnd/peernotifier"
	"github.com/cryptomeow/lnd/routing"
	"github.com/cryptomeow/lnd/routing/localchans"
	"github.com/cryptomeow/lnd/signal"
	"github.com/cryptomeow/lnd/sweep"
	"github.com/cryptomeow/lnd/watchtower"
	"github.com/cryptomeow/lnd/watchtower/wtclient"
	"google.golang.org/grpc"
)

// replaceableLogger is a thin wrapper around a logger that is used so the
// logger can be replaced easily without some black pointer magic.
type replaceableLogger struct {
	btclog.Logger
	subsystem string
}

// Loggers can not be used before the log rotator has been initialized with a
// log file. This must be performed early during application startup by
// calling InitLogRotator() on the main log writer instance in the config.
var (
	// lndPkgLoggers is a list of all lnd package level loggers that are
	// registered. They are tracked here so they can be replaced once the
	// SetupLoggers function is called with the final root logger.
	lndPkgLoggers []*replaceableLogger

	// addLndPkgLogger is a helper function that creates a new replaceable
	// main lnd package level logger and adds it to the list of loggers that
	// are replaced again later, once the final root logger is ready.
	addLndPkgLogger = func(subsystem string) *replaceableLogger {
		l := &replaceableLogger{
			Logger:    build.NewSubLogger(subsystem, nil),
			subsystem: subsystem,
		}
		lndPkgLoggers = append(lndPkgLoggers, l)
		return l
	}

	// Loggers that need to be accessible from the lnd package can be placed
	// here. Loggers that are only used in sub modules can be added directly
	// by using the addSubLogger method. We declare all loggers so we never
	// run into a nil reference if they are used early. But the SetupLoggers
	// function should always be called as soon as possible to finish
	// setting them up properly with a root logger.
	ltndLog = addLndPkgLogger("LTND")
	rpcsLog = addLndPkgLogger("RPCS")
	srvrLog = addLndPkgLogger("SRVR")
	fndgLog = addLndPkgLogger("FNDG")
	utxnLog = addLndPkgLogger("UTXN")
	brarLog = addLndPkgLogger("BRAR")
	atplLog = addLndPkgLogger("ATPL")
)

// SetupLoggers initializes all package-global logger variables.
func SetupLoggers(root *build.RotatingLogWriter) {
	// Now that we have the proper root logger, we can replace the
	// placeholder lnd package loggers.
	for _, l := range lndPkgLoggers {
		l.Logger = build.NewSubLogger(l.subsystem, root.GenSubLogger)
		SetSubLogger(root, l.subsystem, l.Logger)
	}

	// Some of the loggers declared in the main lnd package are also used
	// in sub packages.
	signal.UseLogger(ltndLog)
	autopilot.UseLogger(atplLog)

	AddSubLogger(root, "LNWL", lnwallet.UseLogger)
	AddSubLogger(root, "DISC", discovery.UseLogger)
	AddSubLogger(root, "NTFN", chainntnfs.UseLogger)
	AddSubLogger(root, "CHDB", channeldb.UseLogger)
	AddSubLogger(root, "HSWC", htlcswitch.UseLogger)
	AddSubLogger(root, "CMGR", connmgr.UseLogger)
	AddSubLogger(root, "BTCN", neutrino.UseLogger)
	AddSubLogger(root, "CNCT", contractcourt.UseLogger)
	AddSubLogger(root, "SPHX", sphinx.UseLogger)
	AddSubLogger(root, "SWPR", sweep.UseLogger)
	AddSubLogger(root, "SGNR", signrpc.UseLogger)
	AddSubLogger(root, "WLKT", walletrpc.UseLogger)
	AddSubLogger(root, "ARPC", autopilotrpc.UseLogger)
	AddSubLogger(root, "INVC", invoices.UseLogger)
	AddSubLogger(root, "NANN", netann.UseLogger)
	AddSubLogger(root, "WTWR", watchtower.UseLogger)
	AddSubLogger(root, "NTFR", chainrpc.UseLogger)
	AddSubLogger(root, "IRPC", invoicesrpc.UseLogger)
	AddSubLogger(root, "CHNF", channelnotifier.UseLogger)
	AddSubLogger(root, "CHBU", chanbackup.UseLogger)
	AddSubLogger(root, "PROM", monitoring.UseLogger)
	AddSubLogger(root, "WTCL", wtclient.UseLogger)
	AddSubLogger(root, "PRNF", peernotifier.UseLogger)
	AddSubLogger(root, "CHFD", chanfunding.UseLogger)
	AddSubLogger(root, "PEER", peer.UseLogger)
	AddSubLogger(root, "CHCL", chancloser.UseLogger)

	AddSubLogger(root, routing.Subsystem, routing.UseLogger, localchans.UseLogger)
	AddSubLogger(root, routerrpc.Subsystem, routerrpc.UseLogger)
	AddSubLogger(root, chanfitness.Subsystem, chanfitness.UseLogger)
	AddSubLogger(root, verrpc.Subsystem, verrpc.UseLogger)
	AddSubLogger(root, healthcheck.Subsystem, healthcheck.UseLogger)
}

// AddSubLogger is a helper method to conveniently create and register the
// logger of one or more sub systems.
func AddSubLogger(root *build.RotatingLogWriter, subsystem string,
	useLoggers ...func(btclog.Logger)) {

	// Create and register just a single logger to prevent them from
	// overwriting each other internally.
	logger := build.NewSubLogger(subsystem, root.GenSubLogger)
	SetSubLogger(root, subsystem, logger, useLoggers...)
}

// SetSubLogger is a helper method to conveniently register the logger of a sub
// system.
func SetSubLogger(root *build.RotatingLogWriter, subsystem string,
	logger btclog.Logger, useLoggers ...func(btclog.Logger)) {

	root.RegisterSubLogger(subsystem, logger)
	for _, useLogger := range useLoggers {
		useLogger(logger)
	}
}

// logClosure is used to provide a closure over expensive logging operations so
// don't have to be performed when the logging level doesn't warrant it.
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

// errorLogUnaryServerInterceptor is a simple UnaryServerInterceptor that will
// automatically log any errors that occur when serving a client's unary
// request.
func errorLogUnaryServerInterceptor(logger btclog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler) (interface{}, error) {

		resp, err := handler(ctx, req)
		if err != nil {
			// TODO(roasbeef): also log request details?
			logger.Errorf("[%v]: %v", info.FullMethod, err)
		}

		return resp, err
	}
}

// errorLogStreamServerInterceptor is a simple StreamServerInterceptor that
// will log any errors that occur while processing a client or server streaming
// RPC.
func errorLogStreamServerInterceptor(logger btclog.Logger) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream,
		info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {

		err := handler(srv, ss)
		if err != nil {
			logger.Errorf("[%v]: %v", info.FullMethod, err)
		}

		return err
	}
}
