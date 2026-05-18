package inmem

import (
	"context"
	"testing"

	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

func TestProvider_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := New(DefaultCapacity(), Faults{})

	// Quota at start: zero usage.
	q, err := p.GetQuotaSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetQuotaSnapshot: %v", err)
	}
	if q.MaxUtilizationPct() != 0 {
		t.Errorf("expected 0%% utilization at start, got %v", q.MaxUtilizationPct())
	}

	// Create a network.
	netID, err := p.CreateNetwork(ctx, "proj-1", ports.NetworkSpec{Name: "lab-net", CIDR: "192.168.50.0/24"})
	if err != nil {
		t.Fatalf("CreateNetwork: %v", err)
	}
	if netID == "" {
		t.Errorf("expected non-empty network id")
	}

	// CreateNetwork twice with same args is idempotent.
	netID2, _ := p.CreateNetwork(ctx, "proj-1", ports.NetworkSpec{Name: "lab-net", CIDR: "192.168.50.0/24"})
	if netID != netID2 {
		t.Errorf("expected idempotent CreateNetwork, got %s vs %s", netID, netID2)
	}

	// Create a keypair.
	kp, err := p.CreateKeypair(ctx, "proj-1", "lab-key")
	if err != nil {
		t.Fatalf("CreateKeypair: %v", err)
	}
	if len(kp.PrivateKey) == 0 || kp.PublicKey == "" {
		t.Errorf("keypair has empty material")
	}

	// Boot a server.
	srvID, err := p.BootServer(ctx, "proj-1", ports.ServerSpec{
		Name:        "lab-vm",
		ImageRef:    "ubuntu-22.04",
		FlavorRef:   "m1.small",
		NetworkID:   netID,
		KeypairName: kp.Name,
	})
	if err != nil {
		t.Fatalf("BootServer: %v", err)
	}

	// BootServer is idempotent.
	srvID2, _ := p.BootServer(ctx, "proj-1", ports.ServerSpec{Name: "lab-vm", FlavorRef: "m1.small"})
	if srvID != srvID2 {
		t.Errorf("expected idempotent BootServer, got %s vs %s", srvID, srvID2)
	}

	if err := p.WaitForActive(ctx, "proj-1", srvID); err != nil {
		t.Fatalf("WaitForActive: %v", err)
	}

	ip, err := p.AllocateFloatingIP(ctx, "proj-1", srvID)
	if err != nil {
		t.Fatalf("AllocateFloatingIP: %v", err)
	}
	if ip == "" {
		t.Errorf("expected non-empty IP")
	}

	// Quota now reflects the running server.
	q, _ = p.GetQuotaSnapshot(ctx)
	if q.VCPUs.Used == 0 {
		t.Errorf("expected non-zero CPU usage after boot")
	}

	// Cleanup.
	if err := p.DeleteServer(ctx, "proj-1", srvID); err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}
	if err := p.DeleteKeypair(ctx, "proj-1", kp.Name); err != nil {
		t.Fatalf("DeleteKeypair: %v", err)
	}
	if err := p.DeleteNetwork(ctx, "proj-1", netID); err != nil {
		t.Fatalf("DeleteNetwork: %v", err)
	}

	// Idempotent delete: not-found is not an error.
	if err := p.DeleteServer(ctx, "proj-1", srvID); err != nil {
		t.Errorf("re-deleting server: %v", err)
	}

	q, _ = p.GetQuotaSnapshot(ctx)
	if q.VCPUs.Used != 0 || q.RAM.Used != 0 || q.Disk.Used != 0 {
		t.Errorf("expected zero usage after cleanup, got %+v", q)
	}
}

func TestProvider_CapacityExhaustion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Tiny capacity: only 2 vCPU available.
	p := New(Capacity{VCPUs: 2, RAMMB: 4096, DiskGB: 40}, Faults{})

	for i := 0; i < 2; i++ {
		_, err := p.BootServer(ctx, "proj", ports.ServerSpec{Name: name(i), FlavorRef: "m1.small"})
		if err != nil {
			t.Fatalf("boot %d: %v", i, err)
		}
	}
	// 3rd should fail.
	_, err := p.BootServer(ctx, "proj", ports.ServerSpec{Name: "third", FlavorRef: "m1.small"})
	if err == nil {
		t.Errorf("expected capacity exhausted, got nil")
	}
}

func TestProvider_FaultInjection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	wantErr := contextDeadlineMimic{}
	p := New(DefaultCapacity(), Faults{BootServerErr: wantErr})

	_, err := p.BootServer(ctx, "proj", ports.ServerSpec{Name: "x", FlavorRef: "m1.small"})
	if err != wantErr {
		t.Errorf("expected injected error, got %v", err)
	}
}

type contextDeadlineMimic struct{}

func (contextDeadlineMimic) Error() string { return "simulated boot failure" }

func name(i int) string {
	return "vm-" + string(rune('a'+i))
}
