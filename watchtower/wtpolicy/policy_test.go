package wtpolicy_test

import (
	"testing"

	"github.com/cryptomeow/lnd/watchtower/blob"
	"github.com/cryptomeow/lnd/watchtower/wtpolicy"
)

var validationTests = []struct {
	name   string
	policy wtpolicy.Policy
	expErr error
}{
	{
		name: "fail no maxupdates",
		policy: wtpolicy.Policy{
			TxPolicy: wtpolicy.TxPolicy{
				BlobType: blob.TypeAltruistCommit,
			},
		},
		expErr: wtpolicy.ErrNoMaxUpdates,
	},
	{
		name: "fail altruist with reward base",
		policy: wtpolicy.Policy{
			TxPolicy: wtpolicy.TxPolicy{
				BlobType:   blob.TypeAltruistCommit,
				RewardBase: 1,
			},
		},
		expErr: wtpolicy.ErrAltruistReward,
	},
	{
		name: "fail altruist with reward rate",
		policy: wtpolicy.Policy{
			TxPolicy: wtpolicy.TxPolicy{
				BlobType:   blob.TypeAltruistCommit,
				RewardRate: 1,
			},
		},
		expErr: wtpolicy.ErrAltruistReward,
	},
	{
		name: "fail sweep fee rate too low",
		policy: wtpolicy.Policy{
			TxPolicy: wtpolicy.TxPolicy{
				BlobType: blob.TypeAltruistCommit,
			},
			MaxUpdates: 1,
		},
		expErr: wtpolicy.ErrSweepFeeRateTooLow,
	},
	{
		name: "minimal valid altruist policy",
		policy: wtpolicy.Policy{
			TxPolicy: wtpolicy.TxPolicy{
				BlobType:     blob.TypeAltruistCommit,
				SweepFeeRate: wtpolicy.MinSweepFeeRate,
			},
			MaxUpdates: 1,
		},
	},
	{
		name: "valid altruist policy with default sweep rate",
		policy: wtpolicy.Policy{
			TxPolicy: wtpolicy.TxPolicy{
				BlobType:     blob.TypeAltruistCommit,
				SweepFeeRate: wtpolicy.DefaultSweepFeeRate,
			},
			MaxUpdates: 1,
		},
	},
	{
		name:   "valid default policy",
		policy: wtpolicy.DefaultPolicy(),
	},
}

// TestPolicyValidate asserts that the sanity checks for policies behave as
// expected.
func TestPolicyValidate(t *testing.T) {
	for i := range validationTests {
		test := validationTests[i]
		t.Run(test.name, func(t *testing.T) {
			err := test.policy.Validate()
			if err != test.expErr {
				t.Fatalf("validation error mismatch, "+
					"want: %v, got: %v", test.expErr, err)
			}
		})
	}
}
