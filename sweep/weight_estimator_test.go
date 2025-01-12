package sweep

import (
	"testing"

	"github.com/btcsuite/btcd/wire"
	"github.com/cryptomeow/lnd/input"
	"github.com/cryptomeow/lnd/lnwallet/chainfee"
	"github.com/stretchr/testify/require"
)

// TestWeightEstimator tests weight estimation for inputs with and without
// unconfirmed parents.
func TestWeightEstimator(t *testing.T) {
	testFeeRate := chainfee.SatPerKWeight(20000)

	w := newWeightEstimator(testFeeRate)

	// Add an input without unconfirmed parent tx.
	input1 := input.MakeBaseInput(
		&wire.OutPoint{}, input.CommitmentAnchor,
		&input.SignDescriptor{}, 0, nil,
	)

	require.NoError(t, w.add(&input1))

	// The expectations is that this input is added.
	const expectedWeight1 = 322
	require.Equal(t, expectedWeight1, w.weight())
	require.Equal(t, testFeeRate.FeeForWeight(expectedWeight1), w.fee())

	// Define a parent transaction that pays a fee of 30000 sat/kw.
	parentTxHighFee := &input.TxInfo{
		Weight: 100,
		Fee:    3000,
	}

	// Add an output of the parent tx above.
	input2 := input.MakeBaseInput(
		&wire.OutPoint{}, input.CommitmentAnchor,
		&input.SignDescriptor{}, 0,
		parentTxHighFee,
	)

	require.NoError(t, w.add(&input2))

	// Pay for parent isn't possible because the parent pays a higher fee
	// rate than the child. We expect no additional fee on the child.
	const expectedWeight2 = expectedWeight1 + 280
	require.Equal(t, expectedWeight2, w.weight())
	require.Equal(t, testFeeRate.FeeForWeight(expectedWeight2), w.fee())

	// Define a parent transaction that pays a fee of 10000 sat/kw.
	parentTxLowFee := &input.TxInfo{
		Weight: 100,
		Fee:    1000,
	}

	// Add an output of the low-fee parent tx above.
	input3 := input.MakeBaseInput(
		&wire.OutPoint{}, input.CommitmentAnchor,
		&input.SignDescriptor{}, 0,
		parentTxLowFee,
	)
	require.NoError(t, w.add(&input3))

	// Expect the weight to increase because of the third input.
	const expectedWeight3 = expectedWeight2 + 280
	require.Equal(t, expectedWeight3, w.weight())

	// Expect the fee to cover the child and the parent transaction at 20
	// sat/kw after subtraction of the fee that was already paid by the
	// parent.
	expectedFee := testFeeRate.FeeForWeight(
		expectedWeight3+parentTxLowFee.Weight,
	) - parentTxLowFee.Fee

	require.Equal(t, expectedFee, w.fee())
}
