package deploy

import (
	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

// vmSpec describes one VM in a lab topology.
type vmSpec struct {
	Name   string
	Image  string
	Flavor string
}

// topology is the provider-agnostic description of what a lab template
// deploys: an isolated network plus one or more VMs.
type topology struct {
	NetworkCIDR string
	VMs         []vmSpec
}

// topologyFor returns the topology of a lab template.
//
// TODO: read this from the persisted lab_templates.topology JSONB column once
// a LabTemplate repository exists. Until then every template deploys the same
// single-VM stand, which is enough to exercise the saga end to end.
func topologyFor(_ shared.LabTemplateID) topology {
	return topology{
		NetworkCIDR: "192.168.50.0/24",
		VMs: []vmSpec{
			{Name: "vm", Image: "ubuntu-22.04", Flavor: "m1.small"},
		},
	}
}

// Provider resource names are derived deterministically from the lab ID so
// that re-running a saga step finds (and reuses) what a previous attempt
// already created — this is what makes each step idempotent.
func keypairName(l *labdomain.LabInstance) string { return "lab-" + l.ID.String() + "-key" }
func networkName(l *labdomain.LabInstance) string { return "lab-" + l.ID.String() + "-net" }
func serverName(l *labdomain.LabInstance, vm string) string {
	return "lab-" + l.ID.String() + "-" + vm
}

// zeroize overwrites a byte slice — used to scrub SSH private key material
// from memory once it has been handed to the SecretStore.
func zeroize(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
