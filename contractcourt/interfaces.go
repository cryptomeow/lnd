package contractcourt

import (
	"io"

	"github.com/btcsuite/btcd/wire"
	"github.com/cryptomeow/lnd/channeldb"
	"github.com/cryptomeow/lnd/htlcswitch/hop"
	"github.com/cryptomeow/lnd/input"
	"github.com/cryptomeow/lnd/invoices"
	"github.com/cryptomeow/lnd/lntypes"
	"github.com/cryptomeow/lnd/lnwallet/chainfee"
	"github.com/cryptomeow/lnd/lnwire"
	"github.com/cryptomeow/lnd/sweep"
)

// Registry is an interface which represents the invoice registry.
type Registry interface {
	// LookupInvoice attempts to look up an invoice according to its 32
	// byte payment hash.
	LookupInvoice(lntypes.Hash) (channeldb.Invoice, error)

	// NotifyExitHopHtlc attempts to mark an invoice as settled. If the
	// invoice is a debug invoice, then this method is a noop as debug
	// invoices are never fully settled. The return value describes how the
	// htlc should be resolved. If the htlc cannot be resolved immediately,
	// the resolution is sent on the passed in hodlChan later.
	NotifyExitHopHtlc(payHash lntypes.Hash, paidAmount lnwire.MilliSatoshi,
		expiry uint32, currentHeight int32,
		circuitKey channeldb.CircuitKey, hodlChan chan<- interface{},
		payload invoices.Payload) (invoices.HtlcResolution, error)

	// HodlUnsubscribeAll unsubscribes from all htlc resolutions.
	HodlUnsubscribeAll(subscriber chan<- interface{})
}

// OnionProcessor is an interface used to decode onion blobs.
type OnionProcessor interface {
	// ReconstructHopIterator attempts to decode a valid sphinx packet from
	// the passed io.Reader instance.
	ReconstructHopIterator(r io.Reader, rHash []byte) (hop.Iterator, error)
}

// UtxoSweeper defines the sweep functions that contract court requires.
type UtxoSweeper interface {
	// SweepInput sweeps inputs back into the wallet.
	SweepInput(input input.Input, params sweep.Params) (chan sweep.Result,
		error)

	// CreateSweepTx accepts a list of inputs and signs and generates a txn
	// that spends from them. This method also makes an accurate fee
	// estimate before generating the required witnesses.
	CreateSweepTx(inputs []input.Input, feePref sweep.FeePreference,
		currentBlockHeight uint32) (*wire.MsgTx, error)

	// RelayFeePerKW returns the minimum fee rate required for transactions
	// to be relayed.
	RelayFeePerKW() chainfee.SatPerKWeight

	// UpdateParams allows updating the sweep parameters of a pending input
	// in the UtxoSweeper. This function can be used to provide an updated
	// fee preference that will be used for a new sweep transaction of the
	// input that will act as a replacement transaction (RBF) of the
	// original sweeping transaction, if any.
	UpdateParams(input wire.OutPoint, params sweep.ParamsUpdate) (
		chan sweep.Result, error)
}
