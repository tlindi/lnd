package lncfg

import (
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/lightningnetwork/lnd/contractcourt"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
	"github.com/lightningnetwork/lnd/sweep"
)

const (
	// MaxFeeRateFloor is the smallest config value allowed for the max fee
	// rate in sat/vb.
	MaxFeeRateFloor chainfee.SatPerVByte = 100

	// MaxAllowedFeeRate is the largest fee rate in sat/vb that we allow
	// when configuring the MaxFeeRate.
	MaxAllowedFeeRate = 10_000

	// MinBudgetValue is the minimal budget that we allow when configuring
	// the budget used in sweeping outputs. The actual budget can be lower
	// if the user decides to NOT set this value.
	//
	// NOTE: This value is chosen so the liner fee function can increase at
	// least 1 sat/kw per block.
	MinBudgetValue btcutil.Amount = 1008

	// MinBudgetRatio is the minimal ratio that we allow when configuring
	// the budget ratio used in sweeping outputs.
	MinBudgetRatio = 0.001
)

//nolint:lll
type Sweeper struct {
	BatchWindowDuration time.Duration        `long:"batchwindowduration" description:"Duration of the sweep batch window. The sweep is held back during the batch window to allow more inputs to be added and thereby lower the fee per input." hidden:"true"`
	MaxFeeRate          chainfee.SatPerVByte `long:"maxfeerate" description:"Maximum fee rate in sat/vb that the sweeper is allowed to use when sweeping funds, the fee rate derived from budgets are capped at this value. Setting this value too low can result in transactions not being confirmed in time, causing HTLCs to expire hence potentially losing funds."`

	NoDeadlineConfTarget uint32 `long:"nodeadlineconftarget" description:"The conf target to use when sweeping non-time-sensitive outputs. This is useful for sweeping outputs that are not time-sensitive, and can be swept at a lower fee rate."`

	Budget *contractcourt.BudgetConfig `group:"sweeper.budget" namespace:"budget" long:"budget" description:"An optional config group that's used for the automatic sweep fee estimation. The budget specifies the max fees can be used when sweeping txns, not necessarily the actual fee rate used, and the fee rate calculated from budgets is capped at maxfeerate. Check the budget config options for more details."`
}

// Validate checks the values configured for the sweeper.
func (s *Sweeper) Validate() error {
	if s.BatchWindowDuration < 0 {
		return fmt.Errorf("batchwindowduration must be positive")
	}

	// We require the max fee rate to be at least 100 sat/vbyte.
	if s.MaxFeeRate < MaxFeeRateFloor {
		return fmt.Errorf("maxfeerate must be >= 100 sat/vb")
	}

	// We require the max fee rate to be no greater than 10_000 sat/vbyte.
	if s.MaxFeeRate > MaxAllowedFeeRate {
		return fmt.Errorf("maxfeerate must be <= 10000 sat/vb")
	}

	// Make sure the conf target is at least 144 blocks (1 day).
	if s.NoDeadlineConfTarget < 144 {
		return fmt.Errorf("nodeadlineconftarget must be at least 144")
	}

	// Validate the budget configuration.
	if err := s.Budget.Validate(); err != nil {
		return fmt.Errorf("invalid budget config: %w", err)
	}

	return nil
}

// DefaultSweeperConfig returns the default configuration for the sweeper.
func DefaultSweeperConfig() *Sweeper {
	return &Sweeper{
		MaxFeeRate:           sweep.DefaultMaxFeeRate,
		NoDeadlineConfTarget: uint32(sweep.DefaultDeadlineDelta),
		Budget:               contractcourt.DefaultBudgetConfig(),
	}
}
