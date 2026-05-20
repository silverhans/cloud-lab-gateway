package deploy

import (
	"testing"

	"github.com/google/uuid"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

// TestTopologyFor_LabStand3 pins the hard-coded "лабораторный стенд №3"
// topology so an accidental edit to topologyFor is caught. Once topologyFor
// reads from a persisted lab_templates row this test moves with it.
func TestTopologyFor_LabStand3(t *testing.T) {
	t.Parallel()
	top := topologyFor(shared.LabTemplateID(uuid.New()))

	if top.NetworkCIDR != "10.0.0.0/24" {
		t.Errorf("NetworkCIDR = %q, want 10.0.0.0/24", top.NetworkCIDR)
	}
	if len(top.VMs) != 5 {
		t.Fatalf("VM count = %d, want 5", len(top.VMs))
	}

	want := map[string]vmSpec{
		"L-MS":     {Image: "LAB2_TMP_L-MS", Flavor: "small", IP: "10.0.0.10"},
		"L-NFS":    {Image: "LAB2_TMP_L-NFS", Flavor: "start", IP: "10.0.0.70"},
		"L-PGSQL":  {Image: "LAB2_TMP_L-PGSQL", Flavor: "start", IP: "10.0.0.55"},
		"W-DC":     {Image: "LAB2_TMP_W-DC", Flavor: "medium", IP: "10.0.0.5"},
		"V-HYPERV": {Image: "LAB2_TMP_V-HYPERV", Flavor: "large", IP: "10.0.0.65"},
	}
	seen := make(map[string]bool, len(want))
	for _, vm := range top.VMs {
		w, ok := want[vm.Name]
		if !ok {
			t.Errorf("unexpected VM %q", vm.Name)
			continue
		}
		seen[vm.Name] = true
		if vm.Image != w.Image || vm.Flavor != w.Flavor || vm.IP != w.IP {
			t.Errorf("VM %s = {image:%q flavor:%q ip:%q}, want {image:%q flavor:%q ip:%q}",
				vm.Name, vm.Image, vm.Flavor, vm.IP, w.Image, w.Flavor, w.IP)
		}
		if vm.Role == "" {
			t.Errorf("VM %s has an empty Role", vm.Name)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("missing VM %q", name)
		}
	}
}
