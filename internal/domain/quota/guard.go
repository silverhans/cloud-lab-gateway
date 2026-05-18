// Package quota is the Quota Guard bounded context. It evaluates whether a
// proposed deploy would push КИ cluster utilization above the configured
// threshold (default 90%).
package quota

import (
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

// DefaultThresholdPct is the safety threshold for max resource utilization.
const DefaultThresholdPct = 90.0

// Decision is the result of a quota guard evaluation.
type Decision struct {
	Allowed       bool
	Reason        string
	Snapshot      shared.QuotaSnapshot
	PredictedPct  float64
	ThresholdPct  float64
}

// Evaluate returns whether a deploy that requests `req` is allowed given the
// current snapshot. Pure function — no side effects.
func Evaluate(snap shared.QuotaSnapshot, req shared.ResourceRequest, thresholdPct float64) Decision {
	if thresholdPct <= 0 {
		thresholdPct = DefaultThresholdPct
	}
	predicted := snap.PredictAfter(req)
	pct := predicted.MaxUtilizationPct()
	d := Decision{
		Snapshot:     snap,
		PredictedPct: pct,
		ThresholdPct: thresholdPct,
	}
	if pct > thresholdPct {
		d.Allowed = false
		d.Reason = "predicted utilization exceeds threshold"
		return d
	}
	d.Allowed = true
	return d
}
