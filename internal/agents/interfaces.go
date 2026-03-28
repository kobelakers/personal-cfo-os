package agents

import "github.com/kobelakers/personal-cfo-os/internal/protocol"

type DomainAgent interface {
	Name() string
	Handle(message protocol.AgentEnvelope) error
}

type SystemAgent interface {
	Name() string
	Handle(message protocol.AgentEnvelope) error
}

type CashflowAgent interface{ DomainAgent }
type DebtAgent interface{ DomainAgent }
type TaxAgent interface{ DomainAgent }
type PortfolioAgent interface{ DomainAgent }
type BehaviorAgent interface{ DomainAgent }

type PlannerAgent interface{ SystemAgent }
type MemorySteward interface{ SystemAgent }
type VerificationAgent interface{ SystemAgent }
type GovernanceAgent interface{ SystemAgent }
type ReportAgent interface{ SystemAgent }
