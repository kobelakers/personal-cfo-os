package governance

import (
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/state"
)

type RiskAssessment struct {
	Level   ActionRiskLevel `json:"level"`
	Reasons []string        `json:"reasons,omitempty"`
}

type DefaultRiskClassifier struct{}

func (DefaultRiskClassifier) Classify(current state.FinancialWorldState, action string) RiskAssessment {
	reasons := make([]string, 0, 4)
	level := ActionRiskLow

	if current.LiabilityState.DebtBurdenRatio >= 0.35 || current.LiabilityState.MinimumPaymentPressure >= 0.2 {
		level = ActionRiskHigh
		reasons = append(reasons, "debt pressure is high")
	} else if current.LiabilityState.DebtBurdenRatio >= 0.2 {
		level = ActionRiskMedium
		reasons = append(reasons, "debt burden is elevated")
	}

	if current.RiskState.OverallRisk == "high" {
		level = ActionRiskHigh
		reasons = append(reasons, "overall financial risk is high")
	} else if current.RiskState.OverallRisk == "medium" && level == ActionRiskLow {
		level = ActionRiskMedium
		reasons = append(reasons, "overall financial risk is medium")
	}

	if strings.Contains(strings.ToLower(action), "invest") || strings.Contains(strings.ToLower(action), "stock") {
		if level == ActionRiskMedium {
			level = ActionRiskHigh
		} else if level == ActionRiskLow {
			level = ActionRiskMedium
		}
		reasons = append(reasons, "investment actions carry higher execution risk")
	}

	if current.BehaviorState.LateNightSpendingFrequency >= 0.3 && level == ActionRiskLow {
		level = ActionRiskMedium
		reasons = append(reasons, "behavior signal indicates spending volatility")
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "risk baseline is low")
	}
	return RiskAssessment{Level: level, Reasons: reasons}
}

type ApprovalDecider struct {
	PolicyEngine PolicyEngine
}

func (d ApprovalDecider) Decide(request ActionRequest, approval ApprovalPolicy, toolPolicy *ToolExecutionPolicy) (PolicyDecision, AuditEvent, error) {
	engine := d.PolicyEngine
	if engine == nil {
		engine = StaticPolicyEngine{}
	}
	return engine.EvaluateAction(request, approval, toolPolicy)
}
