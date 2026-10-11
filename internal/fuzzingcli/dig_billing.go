package fuzzingcli

import (
	"hackme/internal/fuzzescrow"
)

// DigBillingQuote is the customer/miner-facing money card for a Dig SKU.
type DigBillingQuote struct {
	Package          string  `json:"package"`
	ProductMode      string  `json:"product_mode"`
	BudgetHMC        float64 `json:"budget_hmc"`
	BudgetRuns       int     `json:"budget_runs"`
	BudgetSeconds    int     `json:"budget_seconds"`
	EscrowSplit      string  `json:"escrow_split"`
	RunsPoolShare    float64 `json:"runs_pool_share"`
	BountyPoolShare  float64 `json:"bounty_pool_share"`
	EstPerRunHMC     float64 `json:"est_per_run_hmc"`
	PoHRewardHMC     float64 `json:"poh_reward_hmc,omitempty"`
	SeedsMerged      bool    `json:"seeds_merged_allowed"`
	BillingModel     string  `json:"billing_model"` // smoke_runs | deep_depth
	HonestyNote      string  `json:"honesty_note"`
}

// QuoteDigBilling builds the money card for a Dig package (pre-order quote).
func QuoteDigBilling(pkg B2BPackage) DigBillingQuote {
	split := fuzzescrow.EscrowSplitForDigPackage(pkg.Name)
	runsShare := fuzzescrow.RunsPoolShare
	bountyShare := fuzzescrow.BountyPoolShare
	model := "smoke_runs"
	note := "Smoke Dig: customer pays for attested distributed smoke; miners earn from 20% runs pool + bounty on findings."
	if IsDeepDigPackage(pkg.Name) {
		runsShare = fuzzescrow.DeepRunsPoolShare
		bountyShare = fuzzescrow.DeepBountyPoolShare
		model = "deep_depth"
		note = "Deep Dig: customer pays for paid depth (higher escrow + 40% runs pool); seed merge on; still not a CVE warranty."
	}
	est := 0.0
	if pkg.BudgetRuns > 0 {
		est = (pkg.BudgetHMC * runsShare) / float64(pkg.BudgetRuns)
	}
	return DigBillingQuote{
		Package:         pkg.Name,
		ProductMode:     DigProductMode(pkg.Name),
		BudgetHMC:       pkg.BudgetHMC,
		BudgetRuns:      pkg.BudgetRuns,
		BudgetSeconds:   pkg.BudgetSeconds,
		EscrowSplit:     split,
		RunsPoolShare:   runsShare,
		BountyPoolShare: bountyShare,
		EstPerRunHMC:    est,
		PoHRewardHMC:    pkg.RewardHMC,
		SeedsMerged:     IsDeepDigPackage(pkg.Name),
		BillingModel:    model,
		HonestyNote:     note,
	}
}
