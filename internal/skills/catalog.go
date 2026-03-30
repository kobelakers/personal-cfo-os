package skills

import (
	"fmt"
	"slices"
	"sort"
)

type SkillCatalog interface {
	Families() []SkillFamily
	Latest(family SkillFamily) (SkillManifest, bool)
	Manifest(family SkillFamily, version SkillVersion) (SkillManifest, bool)
}

type StaticSkillCatalog struct {
	manifests map[SkillFamily]map[SkillVersion]SkillManifest
}

func NewStaticSkillCatalog(manifests ...SkillManifest) (*StaticSkillCatalog, error) {
	items := make(map[SkillFamily]map[SkillVersion]SkillManifest)
	for _, manifest := range manifests {
		if err := manifest.Validate(); err != nil {
			return nil, err
		}
		if _, ok := items[manifest.Family]; !ok {
			items[manifest.Family] = make(map[SkillVersion]SkillManifest)
		}
		if _, ok := items[manifest.Family][manifest.Version]; ok {
			return nil, fmt.Errorf("duplicate skill manifest %s:%s", manifest.Family, manifest.Version)
		}
		items[manifest.Family][manifest.Version] = manifest
	}
	return &StaticSkillCatalog{manifests: items}, nil
}

func (c *StaticSkillCatalog) Families() []SkillFamily {
	if c == nil {
		return nil
	}
	items := make([]SkillFamily, 0, len(c.manifests))
	for family := range c.manifests {
		items = append(items, family)
	}
	sort.Slice(items, func(i, j int) bool { return items[i] < items[j] })
	return items
}

func (c *StaticSkillCatalog) Latest(family SkillFamily) (SkillManifest, bool) {
	if c == nil {
		return SkillManifest{}, false
	}
	versions, ok := c.manifests[family]
	if !ok || len(versions) == 0 {
		return SkillManifest{}, false
	}
	keys := make([]string, 0, len(versions))
	for version := range versions {
		keys = append(keys, string(version))
	}
	sort.Strings(keys)
	return versions[SkillVersion(keys[len(keys)-1])], true
}

func (c *StaticSkillCatalog) Manifest(family SkillFamily, version SkillVersion) (SkillManifest, bool) {
	if c == nil {
		return SkillManifest{}, false
	}
	versions, ok := c.manifests[family]
	if !ok {
		return SkillManifest{}, false
	}
	item, ok := versions[version]
	return item, ok
}

func DefaultBehaviorSkillCatalog() (*StaticSkillCatalog, error) {
	return NewStaticSkillCatalog(
		SkillManifest{
			Family:      SkillFamilySubscriptionCleanup,
			Version:     "v1",
			Title:       "Subscription Cleanup",
			Description: "Surface duplicate or overlapping recurring subscriptions and propose cleanup.",
			Trigger: SkillTrigger{
				Intent:           "behavior_intervention",
				Keywords:         []string{"订阅清理", "subscription cleanup"},
				RequiredEvidence: []string{"transaction_batch", "recurring_subscription_signal"},
			},
			Policy: SkillPolicy{},
			ExpectedOutput: SkillExpectedOutput{
				BlockKind:           "behavior_intervention_block",
				RecommendationTypes: []string{"behavior_subscription_cleanup"},
			},
			Recipes: []SkillRecipe{
				{
					ID:                    "subscription_cleanup.v1",
					Title:                 "Subscription Cleanup v1",
					Description:           "Recommend cancelling overlapping subscriptions and reducing duplicate recurring spend.",
					InterventionIntensity: InterventionIntensityModerate,
					RequiredEvidenceTypes: []string{"transaction_batch", "recurring_subscription_signal"},
					RequiredStateBlocks:   []string{"behavior_state", "cashflow_state"},
				},
			},
		},
		SkillManifest{
			Family:      SkillFamilyLateNightSpendNudge,
			Version:     "v1",
			Title:       "Late-Night Spend Nudge",
			Description: "Address late-night discretionary spend spikes with lighter intervention.",
			Trigger: SkillTrigger{
				Intent:           "behavior_intervention",
				Keywords:         []string{"深夜消费", "late night spending"},
				RequiredEvidence: []string{"transaction_batch", "late_night_spending_signal"},
			},
			Policy: SkillPolicy{},
			ExpectedOutput: SkillExpectedOutput{
				BlockKind:           "behavior_intervention_block",
				RecommendationTypes: []string{"behavior_spend_nudge"},
			},
			Recipes: []SkillRecipe{
				{
					ID:                    "late_night_spend_nudge.v1",
					Title:                 "Late-Night Spend Nudge v1",
					Description:           "Recommend a lighter review and spend pause for late-night discretionary activity.",
					InterventionIntensity: InterventionIntensityLow,
					RequiredEvidenceTypes: []string{"transaction_batch", "late_night_spending_signal"},
					RequiredStateBlocks:   []string{"behavior_state", "cashflow_state"},
				},
			},
		},
		SkillManifest{
			Family:      SkillFamilyDiscretionaryGuardrail,
			Version:     "v1",
			Title:       "Discretionary Guardrail",
			Description: "Escalate discretionary spend interventions based on repeated anomalies and prior outcome memory.",
			Trigger: SkillTrigger{
				Intent:           "behavior_intervention",
				Keywords:         []string{"消费护栏", "behavior intervention", "spending behavior review"},
				RequiredEvidence: []string{"transaction_batch", "late_night_spending_signal"},
			},
			Policy: SkillPolicy{
				RequiresApprovalRecipes: []string{"hard_cap.v1"},
			},
			ExpectedOutput: SkillExpectedOutput{
				BlockKind:           "behavior_intervention_block",
				RecommendationTypes: []string{"behavior_guardrail"},
			},
			Recipes: []SkillRecipe{
				{
					ID:                    "soft_nudge.v1",
					Title:                 "Soft Nudge v1",
					Description:           "Recommend a lightweight discretionary-spend review with minimal intervention.",
					InterventionIntensity: InterventionIntensityLow,
					RequiredEvidenceTypes: []string{"transaction_batch", "late_night_spending_signal"},
					RequiredStateBlocks:   []string{"behavior_state", "cashflow_state"},
				},
				{
					ID:                    "budget_guardrail.v1",
					Title:                 "Budget Guardrail v1",
					Description:           "Recommend a stricter budget guardrail for recurring discretionary pressure.",
					InterventionIntensity: InterventionIntensityModerate,
					RequiredEvidenceTypes: []string{"transaction_batch", "late_night_spending_signal"},
					RequiredStateBlocks:   []string{"behavior_state", "cashflow_state"},
				},
				{
					ID:                    "hard_cap.v1",
					Title:                 "Hard Cap v1",
					Description:           "Recommend a hard discretionary spending cap under repeated pressure. This is governed and approval-gated.",
					InterventionIntensity: InterventionIntensityHigh,
					RequiredEvidenceTypes: []string{"transaction_batch", "late_night_spending_signal"},
					RequiredStateBlocks:   []string{"behavior_state", "cashflow_state", "risk_state"},
					ApprovalRequired:      true,
					PolicyRuleRefs:        []string{"behavior.guardrail.hard_cap.approval_required"},
				},
			},
		},
	)
}

func recipeForID(manifest SkillManifest, recipeID string) (SkillRecipe, bool) {
	idx := slices.IndexFunc(manifest.Recipes, func(item SkillRecipe) bool { return item.ID == recipeID })
	if idx < 0 {
		return SkillRecipe{}, false
	}
	return manifest.Recipes[idx], true
}
