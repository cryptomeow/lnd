package netann

import (
	"net"
	"time"

	"github.com/btcsuite/btcd/btcec"
	"github.com/cryptomeow/lnd/lnwallet"
	"github.com/cryptomeow/lnd/lnwire"
)

// NodeAnnModifier is a closure that makes in-place modifications to an
// lnwire.NodeAnnouncement.
type NodeAnnModifier func(*lnwire.NodeAnnouncement)

// NodeAnnSetAddrs is a functional option that allows updating the addresses of
// the given node announcement.
func NodeAnnSetAddrs(addrs []net.Addr) func(*lnwire.NodeAnnouncement) {
	return func(nodeAnn *lnwire.NodeAnnouncement) {
		nodeAnn.Addresses = addrs
	}
}

// NodeAnnSetTimestamp is a functional option that sets the timestamp of the
// announcement to the current time, or increments it if the timestamp is
// already in the future.
func NodeAnnSetTimestamp(nodeAnn *lnwire.NodeAnnouncement) {
	newTimestamp := uint32(time.Now().Unix())
	if newTimestamp <= nodeAnn.Timestamp {
		// Increment the prior value to  ensure the timestamp
		// monotonically increases, otherwise the announcement won't
		// propagate.
		newTimestamp = nodeAnn.Timestamp + 1
	}
	nodeAnn.Timestamp = newTimestamp
}

// SignNodeAnnouncement applies the given modifies to the passed
// lnwire.NodeAnnouncement, then signs the resulting announcement. The provided
// update should be the most recent, valid update, otherwise the timestamp may
// not monotonically increase from the prior.
func SignNodeAnnouncement(signer lnwallet.MessageSigner,
	pubKey *btcec.PublicKey, nodeAnn *lnwire.NodeAnnouncement,
	mods ...NodeAnnModifier) error {

	// Apply the requested changes to the node announcement.
	for _, modifier := range mods {
		modifier(nodeAnn)
	}

	// Create the DER-encoded ECDSA signature over the message digest.
	sig, err := SignAnnouncement(signer, pubKey, nodeAnn)
	if err != nil {
		return err
	}

	// Parse the DER-encoded signature into a fixed-size 64-byte array.
	nodeAnn.Signature, err = lnwire.NewSigFromSignature(sig)
	return err
}
