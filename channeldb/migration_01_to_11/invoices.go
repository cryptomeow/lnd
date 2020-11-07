package migration_01_to_11

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/btcsuite/btcd/wire"
	"github.com/cryptomeow/lnd/channeldb/kvdb"
	"github.com/cryptomeow/lnd/lntypes"
	"github.com/cryptomeow/lnd/lnwire"
	"github.com/cryptomeow/lnd/tlv"
)

var (

	// invoiceBucket is the name of the bucket within the database that
	// stores all data related to invoices no matter their final state.
	// Within the invoice bucket, each invoice is keyed by its invoice ID
	// which is a monotonically increasing uint32.
	invoiceBucket = []byte("invoices")

	// addIndexBucket is an index bucket that we'll use to create a
	// monotonically increasing set of add indexes. Each time we add a new
	// invoice, this sequence number will be incremented and then populated
	// within the new invoice.
	//
	// In addition to this sequence number, we map:
	//
	//   addIndexNo => invoiceKey
	addIndexBucket = []byte("invoice-add-index")

	// settleIndexBucket is an index bucket that we'll use to create a
	// monotonically increasing integer for tracking a "settle index". Each
	// time an invoice is settled, this sequence number will be incremented
	// as populate within the newly settled invoice.
	//
	// In addition to this sequence number, we map:
	//
	//   settleIndexNo => invoiceKey
	settleIndexBucket = []byte("invoice-settle-index")
)

const (
	// MaxMemoSize is maximum size of the memo field within invoices stored
	// in the database.
	MaxMemoSize = 1024

	// MaxReceiptSize is the maximum size of the payment receipt stored
	// within the database along side incoming/outgoing invoices.
	MaxReceiptSize = 1024

	// MaxPaymentRequestSize is the max size of a payment request for
	// this invoice.
	// TODO(halseth): determine the max length payment request when field
	// lengths are final.
	MaxPaymentRequestSize = 4096

	// A set of tlv type definitions used to serialize invoice htlcs to the
	// database.
	chanIDType       tlv.Type = 1
	htlcIDType       tlv.Type = 3
	amtType          tlv.Type = 5
	acceptHeightType tlv.Type = 7
	acceptTimeType   tlv.Type = 9
	resolveTimeType  tlv.Type = 11
	expiryHeightType tlv.Type = 13
	stateType        tlv.Type = 15
)

// ContractState describes the state the invoice is in.
type ContractState uint8

const (
	// ContractOpen means the invoice has only been created.
	ContractOpen ContractState = 0

	// ContractSettled means the htlc is settled and the invoice has been
	// paid.
	ContractSettled ContractState = 1

	// ContractCanceled means the invoice has been canceled.
	ContractCanceled ContractState = 2

	// ContractAccepted means the HTLC has been accepted but not settled
	// yet.
	ContractAccepted ContractState = 3
)

// String returns a human readable identifier for the ContractState type.
func (c ContractState) String() string {
	switch c {
	case ContractOpen:
		return "Open"
	case ContractSettled:
		return "Settled"
	case ContractCanceled:
		return "Canceled"
	case ContractAccepted:
		return "Accepted"
	}

	return "Unknown"
}

// ContractTerm is a companion struct to the Invoice struct. This struct houses
// the necessary conditions required before the invoice can be considered fully
// settled by the payee.
type ContractTerm struct {
	// PaymentPreimage is the preimage which is to be revealed in the
	// occasion that an HTLC paying to the hash of this preimage is
	// extended.
	PaymentPreimage lntypes.Preimage

	// Value is the expected amount of milli-satoshis to be paid to an HTLC
	// which can be satisfied by the above preimage.
	Value lnwire.MilliSatoshi

	// State describes the state the invoice is in.
	State ContractState
}

// Invoice is a payment invoice generated by a payee in order to request
// payment for some good or service. The inclusion of invoices within Lightning
// creates a payment work flow for merchants very similar to that of the
// existing financial system within PayPal, etc.  Invoices are added to the
// database when a payment is requested, then can be settled manually once the
// payment is received at the upper layer. For record keeping purposes,
// invoices are never deleted from the database, instead a bit is toggled
// denoting the invoice has been fully settled. Within the database, all
// invoices must have a unique payment hash which is generated by taking the
// sha256 of the payment preimage.
type Invoice struct {
	// Memo is an optional memo to be stored along side an invoice.  The
	// memo may contain further details pertaining to the invoice itself,
	// or any other message which fits within the size constraints.
	Memo []byte

	// Receipt is an optional field dedicated for storing a
	// cryptographically binding receipt of payment.
	//
	// TODO(roasbeef): document scheme.
	Receipt []byte

	// PaymentRequest is an optional field where a payment request created
	// for this invoice can be stored.
	PaymentRequest []byte

	// FinalCltvDelta is the minimum required number of blocks before htlc
	// expiry when the invoice is accepted.
	FinalCltvDelta int32

	// Expiry defines how long after creation this invoice should expire.
	Expiry time.Duration

	// CreationDate is the exact time the invoice was created.
	CreationDate time.Time

	// SettleDate is the exact time the invoice was settled.
	SettleDate time.Time

	// Terms are the contractual payment terms of the invoice. Once all the
	// terms have been satisfied by the payer, then the invoice can be
	// considered fully fulfilled.
	//
	// TODO(roasbeef): later allow for multiple terms to fulfill the final
	// invoice: payment fragmentation, etc.
	Terms ContractTerm

	// AddIndex is an auto-incrementing integer that acts as a
	// monotonically increasing sequence number for all invoices created.
	// Clients can then use this field as a "checkpoint" of sorts when
	// implementing a streaming RPC to notify consumers of instances where
	// an invoice has been added before they re-connected.
	//
	// NOTE: This index starts at 1.
	AddIndex uint64

	// SettleIndex is an auto-incrementing integer that acts as a
	// monotonically increasing sequence number for all settled invoices.
	// Clients can then use this field as a "checkpoint" of sorts when
	// implementing a streaming RPC to notify consumers of instances where
	// an invoice has been settled before they re-connected.
	//
	// NOTE: This index starts at 1.
	SettleIndex uint64

	// AmtPaid is the final amount that we ultimately accepted for pay for
	// this invoice. We specify this value independently as it's possible
	// that the invoice originally didn't specify an amount, or the sender
	// overpaid.
	AmtPaid lnwire.MilliSatoshi

	// Htlcs records all htlcs that paid to this invoice. Some of these
	// htlcs may have been marked as canceled.
	Htlcs map[CircuitKey]*InvoiceHTLC
}

// HtlcState defines the states an htlc paying to an invoice can be in.
type HtlcState uint8

// InvoiceHTLC contains details about an htlc paying to this invoice.
type InvoiceHTLC struct {
	// Amt is the amount that is carried by this htlc.
	Amt lnwire.MilliSatoshi

	// AcceptHeight is the block height at which the invoice registry
	// decided to accept this htlc as a payment to the invoice. At this
	// height, the invoice cltv delay must have been met.
	AcceptHeight uint32

	// AcceptTime is the wall clock time at which the invoice registry
	// decided to accept the htlc.
	AcceptTime time.Time

	// ResolveTime is the wall clock time at which the invoice registry
	// decided to settle the htlc.
	ResolveTime time.Time

	// Expiry is the expiry height of this htlc.
	Expiry uint32

	// State indicates the state the invoice htlc is currently in. A
	// canceled htlc isn't just removed from the invoice htlcs map, because
	// we need AcceptHeight to properly cancel the htlc back.
	State HtlcState
}

func validateInvoice(i *Invoice) error {
	if len(i.Memo) > MaxMemoSize {
		return fmt.Errorf("max length a memo is %v, and invoice "+
			"of length %v was provided", MaxMemoSize, len(i.Memo))
	}
	if len(i.Receipt) > MaxReceiptSize {
		return fmt.Errorf("max length a receipt is %v, and invoice "+
			"of length %v was provided", MaxReceiptSize,
			len(i.Receipt))
	}
	if len(i.PaymentRequest) > MaxPaymentRequestSize {
		return fmt.Errorf("max length of payment request is %v, length "+
			"provided was %v", MaxPaymentRequestSize,
			len(i.PaymentRequest))
	}
	return nil
}

// FetchAllInvoices returns all invoices currently stored within the database.
// If the pendingOnly param is true, then only unsettled invoices will be
// returned, skipping all invoices that are fully settled.
func (d *DB) FetchAllInvoices(pendingOnly bool) ([]Invoice, error) {
	var invoices []Invoice

	err := kvdb.View(d, func(tx kvdb.RTx) error {
		invoiceB := tx.ReadBucket(invoiceBucket)
		if invoiceB == nil {
			return ErrNoInvoicesCreated
		}

		// Iterate through the entire key space of the top-level
		// invoice bucket. If key with a non-nil value stores the next
		// invoice ID which maps to the corresponding invoice.
		return invoiceB.ForEach(func(k, v []byte) error {
			if v == nil {
				return nil
			}

			invoiceReader := bytes.NewReader(v)
			invoice, err := deserializeInvoice(invoiceReader)
			if err != nil {
				return err
			}

			if pendingOnly &&
				invoice.Terms.State == ContractSettled {

				return nil
			}

			invoices = append(invoices, invoice)

			return nil
		})
	}, func() {
		invoices = nil
	})
	if err != nil {
		return nil, err
	}

	return invoices, nil
}

// serializeInvoice serializes an invoice to a writer.
//
// Note: this function is in use for a migration. Before making changes that
// would modify the on disk format, make a copy of the original code and store
// it with the migration.
func serializeInvoice(w io.Writer, i *Invoice) error {
	if err := wire.WriteVarBytes(w, 0, i.Memo[:]); err != nil {
		return err
	}
	if err := wire.WriteVarBytes(w, 0, i.Receipt[:]); err != nil {
		return err
	}
	if err := wire.WriteVarBytes(w, 0, i.PaymentRequest[:]); err != nil {
		return err
	}

	if err := binary.Write(w, byteOrder, i.FinalCltvDelta); err != nil {
		return err
	}

	if err := binary.Write(w, byteOrder, int64(i.Expiry)); err != nil {
		return err
	}

	birthBytes, err := i.CreationDate.MarshalBinary()
	if err != nil {
		return err
	}

	if err := wire.WriteVarBytes(w, 0, birthBytes); err != nil {
		return err
	}

	settleBytes, err := i.SettleDate.MarshalBinary()
	if err != nil {
		return err
	}

	if err := wire.WriteVarBytes(w, 0, settleBytes); err != nil {
		return err
	}

	if _, err := w.Write(i.Terms.PaymentPreimage[:]); err != nil {
		return err
	}

	var scratch [8]byte
	byteOrder.PutUint64(scratch[:], uint64(i.Terms.Value))
	if _, err := w.Write(scratch[:]); err != nil {
		return err
	}

	if err := binary.Write(w, byteOrder, i.Terms.State); err != nil {
		return err
	}

	if err := binary.Write(w, byteOrder, i.AddIndex); err != nil {
		return err
	}
	if err := binary.Write(w, byteOrder, i.SettleIndex); err != nil {
		return err
	}
	if err := binary.Write(w, byteOrder, int64(i.AmtPaid)); err != nil {
		return err
	}

	if err := serializeHtlcs(w, i.Htlcs); err != nil {
		return err
	}

	return nil
}

// serializeHtlcs serializes a map containing circuit keys and invoice htlcs to
// a writer.
func serializeHtlcs(w io.Writer, htlcs map[CircuitKey]*InvoiceHTLC) error {
	for key, htlc := range htlcs {
		// Encode the htlc in a tlv stream.
		chanID := key.ChanID.ToUint64()
		amt := uint64(htlc.Amt)
		acceptTime := uint64(htlc.AcceptTime.UnixNano())
		resolveTime := uint64(htlc.ResolveTime.UnixNano())
		state := uint8(htlc.State)

		tlvStream, err := tlv.NewStream(
			tlv.MakePrimitiveRecord(chanIDType, &chanID),
			tlv.MakePrimitiveRecord(htlcIDType, &key.HtlcID),
			tlv.MakePrimitiveRecord(amtType, &amt),
			tlv.MakePrimitiveRecord(
				acceptHeightType, &htlc.AcceptHeight,
			),
			tlv.MakePrimitiveRecord(acceptTimeType, &acceptTime),
			tlv.MakePrimitiveRecord(resolveTimeType, &resolveTime),
			tlv.MakePrimitiveRecord(expiryHeightType, &htlc.Expiry),
			tlv.MakePrimitiveRecord(stateType, &state),
		)
		if err != nil {
			return err
		}

		var b bytes.Buffer
		if err := tlvStream.Encode(&b); err != nil {
			return err
		}

		// Write the length of the tlv stream followed by the stream
		// bytes.
		err = binary.Write(w, byteOrder, uint64(b.Len()))
		if err != nil {
			return err
		}

		if _, err := w.Write(b.Bytes()); err != nil {
			return err
		}
	}

	return nil
}

func deserializeInvoice(r io.Reader) (Invoice, error) {
	var err error
	invoice := Invoice{}

	// TODO(roasbeef): use read full everywhere
	invoice.Memo, err = wire.ReadVarBytes(r, 0, MaxMemoSize, "")
	if err != nil {
		return invoice, err
	}
	invoice.Receipt, err = wire.ReadVarBytes(r, 0, MaxReceiptSize, "")
	if err != nil {
		return invoice, err
	}

	invoice.PaymentRequest, err = wire.ReadVarBytes(r, 0, MaxPaymentRequestSize, "")
	if err != nil {
		return invoice, err
	}

	if err := binary.Read(r, byteOrder, &invoice.FinalCltvDelta); err != nil {
		return invoice, err
	}

	var expiry int64
	if err := binary.Read(r, byteOrder, &expiry); err != nil {
		return invoice, err
	}
	invoice.Expiry = time.Duration(expiry)

	birthBytes, err := wire.ReadVarBytes(r, 0, 300, "birth")
	if err != nil {
		return invoice, err
	}
	if err := invoice.CreationDate.UnmarshalBinary(birthBytes); err != nil {
		return invoice, err
	}

	settledBytes, err := wire.ReadVarBytes(r, 0, 300, "settled")
	if err != nil {
		return invoice, err
	}
	if err := invoice.SettleDate.UnmarshalBinary(settledBytes); err != nil {
		return invoice, err
	}

	if _, err := io.ReadFull(r, invoice.Terms.PaymentPreimage[:]); err != nil {
		return invoice, err
	}
	var scratch [8]byte
	if _, err := io.ReadFull(r, scratch[:]); err != nil {
		return invoice, err
	}
	invoice.Terms.Value = lnwire.MilliSatoshi(byteOrder.Uint64(scratch[:]))

	if err := binary.Read(r, byteOrder, &invoice.Terms.State); err != nil {
		return invoice, err
	}

	if err := binary.Read(r, byteOrder, &invoice.AddIndex); err != nil {
		return invoice, err
	}
	if err := binary.Read(r, byteOrder, &invoice.SettleIndex); err != nil {
		return invoice, err
	}
	if err := binary.Read(r, byteOrder, &invoice.AmtPaid); err != nil {
		return invoice, err
	}

	invoice.Htlcs, err = deserializeHtlcs(r)
	if err != nil {
		return Invoice{}, err
	}

	return invoice, nil
}

// deserializeHtlcs reads a list of invoice htlcs from a reader and returns it
// as a map.
func deserializeHtlcs(r io.Reader) (map[CircuitKey]*InvoiceHTLC, error) {
	htlcs := make(map[CircuitKey]*InvoiceHTLC, 0)

	for {
		// Read the length of the tlv stream for this htlc.
		var streamLen uint64
		if err := binary.Read(r, byteOrder, &streamLen); err != nil {
			if err == io.EOF {
				break
			}

			return nil, err
		}

		streamBytes := make([]byte, streamLen)
		if _, err := r.Read(streamBytes); err != nil {
			return nil, err
		}
		streamReader := bytes.NewReader(streamBytes)

		// Decode the contents into the htlc fields.
		var (
			htlc                    InvoiceHTLC
			key                     CircuitKey
			chanID                  uint64
			state                   uint8
			acceptTime, resolveTime uint64
			amt                     uint64
		)
		tlvStream, err := tlv.NewStream(
			tlv.MakePrimitiveRecord(chanIDType, &chanID),
			tlv.MakePrimitiveRecord(htlcIDType, &key.HtlcID),
			tlv.MakePrimitiveRecord(amtType, &amt),
			tlv.MakePrimitiveRecord(
				acceptHeightType, &htlc.AcceptHeight,
			),
			tlv.MakePrimitiveRecord(acceptTimeType, &acceptTime),
			tlv.MakePrimitiveRecord(resolveTimeType, &resolveTime),
			tlv.MakePrimitiveRecord(expiryHeightType, &htlc.Expiry),
			tlv.MakePrimitiveRecord(stateType, &state),
		)
		if err != nil {
			return nil, err
		}

		if err := tlvStream.Decode(streamReader); err != nil {
			return nil, err
		}

		key.ChanID = lnwire.NewShortChanIDFromInt(chanID)
		htlc.AcceptTime = time.Unix(0, int64(acceptTime))
		htlc.ResolveTime = time.Unix(0, int64(resolveTime))
		htlc.State = HtlcState(state)
		htlc.Amt = lnwire.MilliSatoshi(amt)

		htlcs[key] = &htlc
	}

	return htlcs, nil
}
