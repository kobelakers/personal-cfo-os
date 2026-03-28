package state

type StateReducer interface {
	ApplyEvidencePatch(current FinancialWorldState, patch EvidencePatch) (FinancialWorldState, StateDiff, error)
}
