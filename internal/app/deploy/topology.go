package deploy

import (
	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

// vmSpec describes one VM in a lab topology.
type vmSpec struct {
	Name   string // logical name within the stand, e.g. "W-DC"
	Image  string // КИ Glance image name
	Flavor string // КИ flavor name: start | small | medium | large
	IP     string // fixed address inside the lab network — services bind to it
	Role   string // what the machine does; surfaced to checks and the UI
}

// topology is the provider-agnostic description of what a lab template
// deploys: an isolated network plus one or more VMs.
type topology struct {
	NetworkCIDR string
	VMs         []vmSpec
}

// topologyFor returns the topology of a lab template.
//
// It currently returns one hard-coded stand — "лабораторный стенд №3", a
// Кибер Бэкап / Active Directory environment of five machines on a single
// isolated 10.0.0.0/24 network. The gateway is 10.0.0.1 by convention (first
// host of the CIDR); the cloud adapter wires the router uplink to Public.
//
// The per-VM fixed IPs are part of the topology, not an implementation
// detail: W-DC (10.0.0.5) is the domain's DNS server and every other host
// resolves through it. Until ports.ServerSpec carries a first-class fixed-IP
// field (a coordinated ports change), runBootVM hands the address to the
// adapter via ServerSpec.Metadata.
//
// TODO: read this from the persisted lab_templates.topology JSONB column once
// a LabTemplate repository exists, keyed by the LabTemplateID argument.
func topologyFor(_ shared.LabTemplateID) topology {
	return topology{
		NetworkCIDR: "10.0.0.0/24",
		VMs: []vmSpec{
			{Name: "L-MS", Image: "LAB2_TMP_L-MS", Flavor: "small", IP: "10.0.0.10", Role: "Кибер Бэкап — сервер управления"},
			{Name: "L-NFS", Image: "LAB2_TMP_L-NFS", Flavor: "start", IP: "10.0.0.70", Role: "файловый сервер, хранилище ВМ"},
			{Name: "L-PGSQL", Image: "LAB2_TMP_L-PGSQL", Flavor: "start", IP: "10.0.0.55", Role: "сервер БД PostgreSQL"},
			{Name: "W-DC", Image: "LAB2_TMP_W-DC", Flavor: "medium", IP: "10.0.0.5", Role: "контроллер домена, DNS, SMB"},
			{Name: "V-HYPERV", Image: "LAB2_TMP_V-HYPERV", Flavor: "large", IP: "10.0.0.65", Role: "гипервизор Microsoft Hyper-V"},
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
