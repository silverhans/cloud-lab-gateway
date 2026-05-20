package openstack

import (
	"context"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/limits"
)

type hypervisorStatsResponse struct {
	HypervisorStatistics struct {
		VCPUs        int64 `json:"vcpus"`
		VCPUsUsed    int64 `json:"vcpus_used"`
		MemoryMB     int64 `json:"memory_mb"`
		MemoryMBUsed int64 `json:"memory_mb_used"`
		LocalGB      int64 `json:"local_gb"`
		LocalGBUsed  int64 `json:"local_gb_used"`
	} `json:"hypervisor_statistics"`
}

// GetQuotaSnapshot returns cluster-level utilisation from Nova hypervisor
// statistics plus Cinder limits when available.
func (p *Provider) GetQuotaSnapshot(ctx context.Context) (shared.QuotaSnapshot, error) {
	compute, err := p.compute(ctx, "")
	if err != nil {
		return shared.QuotaSnapshot{}, err
	}

	var stats hypervisorStatsResponse
	_, err = compute.Get(ctx, compute.ServiceURL("os-hypervisors", "statistics"), &stats, nil)
	if err != nil {
		return shared.QuotaSnapshot{}, translateErr("hypervisor statistics", err)
	}

	diskUsed := stats.HypervisorStatistics.LocalGBUsed
	diskTotal := stats.HypervisorStatistics.LocalGB
	if cinder, err := p.blockStorage(ctx, ""); err == nil {
		if lim, err := limits.Get(ctx, cinder).Extract(); err == nil && lim != nil && lim.Absolute.MaxTotalVolumeGigabytes >= 0 {
			// Cinder limits are project-scoped in vanilla OpenStack. КИ may expose
			// a cluster-wide extension; until then Nova local_gb is the fallback.
			diskUsed = int64(lim.Absolute.TotalGigabytesUsed)
			diskTotal = int64(lim.Absolute.MaxTotalVolumeGigabytes)
		}
	}

	return shared.QuotaSnapshot{
		VCPUs: shared.Capacity{
			Used:  stats.HypervisorStatistics.VCPUsUsed,
			Total: stats.HypervisorStatistics.VCPUs,
			Unit:  "vcpu",
		},
		RAM: shared.Capacity{
			Used:  stats.HypervisorStatistics.MemoryMBUsed,
			Total: stats.HypervisorStatistics.MemoryMB,
			Unit:  "MB",
		},
		Disk: shared.Capacity{
			Used:  diskUsed,
			Total: diskTotal,
			Unit:  "GB",
		},
		FetchedAt: time.Now().UTC(),
	}, nil
}
