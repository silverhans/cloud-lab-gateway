package shared

import "time"

// ResourceRequest describes resources required to deploy a lab.
// All fields are non-negative; zero means "no requirement".
type ResourceRequest struct {
	VCPUs  int
	RAMMB  int
	DiskGB int
}

func (r ResourceRequest) Add(o ResourceRequest) ResourceRequest {
	return ResourceRequest{
		VCPUs:  r.VCPUs + o.VCPUs,
		RAMMB:  r.RAMMB + o.RAMMB,
		DiskGB: r.DiskGB + o.DiskGB,
	}
}

// QuotaSnapshot represents КИ cluster utilization at a point in time.
type QuotaSnapshot struct {
	VCPUs     Capacity
	RAM       Capacity
	Disk      Capacity
	FetchedAt time.Time
}

type Capacity struct {
	Used  int64
	Total int64
	Unit  string
}

// UtilizationPct returns used/total as percentage in [0, 100]. Returns 0 when Total is 0.
func (c Capacity) UtilizationPct() float64 {
	if c.Total == 0 {
		return 0
	}
	return float64(c.Used) * 100.0 / float64(c.Total)
}

// MaxUtilizationPct returns the maximum utilization across all three resources.
// This is what the quota guard compares against the threshold.
func (q QuotaSnapshot) MaxUtilizationPct() float64 {
	m := q.VCPUs.UtilizationPct()
	if v := q.RAM.UtilizationPct(); v > m {
		m = v
	}
	if v := q.Disk.UtilizationPct(); v > m {
		m = v
	}
	return m
}

// PredictAfter returns a hypothetical QuotaSnapshot after the given request is
// allocated. Used by QuotaGuard to make a predictive decision before deploy.
func (q QuotaSnapshot) PredictAfter(req ResourceRequest) QuotaSnapshot {
	out := q
	out.VCPUs.Used += int64(req.VCPUs)
	out.RAM.Used += int64(req.RAMMB)
	out.Disk.Used += int64(req.DiskGB)
	return out
}
