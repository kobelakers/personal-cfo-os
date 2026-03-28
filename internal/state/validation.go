package state

import (
	"errors"
	"fmt"
)

func (s FinancialWorldState) Validate() error {
	var errs []error
	if s.UserID == "" {
		errs = append(errs, errors.New("user_id is required"))
	}
	if s.CashflowState.SavingsRate < 0 || s.CashflowState.SavingsRate > 1 {
		errs = append(errs, errors.New("cashflow savings rate must be within [0,1]"))
	}
	if s.LiabilityState.DebtBurdenRatio < 0 || s.LiabilityState.DebtBurdenRatio > 1 {
		errs = append(errs, errors.New("liability debt burden ratio must be within [0,1]"))
	}
	if s.LiabilityState.MinimumPaymentPressure < 0 || s.LiabilityState.MinimumPaymentPressure > 1 {
		errs = append(errs, errors.New("liability minimum payment pressure must be within [0,1]"))
	}
	if s.PortfolioState.EmergencyFundMonths < 0 {
		errs = append(errs, errors.New("emergency fund months must be non-negative"))
	}
	if s.TaxState.EffectiveTaxRate < 0 || s.TaxState.EffectiveTaxRate > 1 {
		errs = append(errs, errors.New("effective tax rate must be within [0,1]"))
	}
	if s.BehaviorState.LateNightSpendingFrequency < 0 || s.BehaviorState.LateNightSpendingFrequency > 1 {
		errs = append(errs, errors.New("late-night spending frequency must be within [0,1]"))
	}
	if s.Version.Sequence == 0 && s.Version.SnapshotID != "" {
		errs = append(errs, fmt.Errorf("snapshot id %q cannot exist on zero sequence", s.Version.SnapshotID))
	}
	return errors.Join(errs...)
}
