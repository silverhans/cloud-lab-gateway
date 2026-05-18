// Package ports contains all interfaces the domain depends on. Outbound
// adapters (cloud, lms, checker, storage, queue, secrets) implement these.
//
// Rules:
//   - ports MUST NOT import adapters
//   - ports may import internal/domain/* and standard library
//   - all methods take context.Context as the first argument
package ports

import (
	"context"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

// CloudProvider abstracts the underlying КИ/OpenStack API. The driver implementation
// in adapters/cloud/openstack uses gophercloud. Tests use adapters/cloud/inmem.
type CloudProvider interface {
	// GetQuotaSnapshot returns hypervisor statistics for the whole cluster.
	// Adapters MUST aggregate hypervisor-statistics or equivalent endpoint;
	// per-project quota is insufficient for the capacity guard.
	GetQuotaSnapshot(ctx context.Context) (shared.QuotaSnapshot, error)

	// CreateKeypair generates an SSH keypair scoped to the given project.
	// Returns the private key material — caller is responsible for encrypting
	// it before persistence.
	CreateKeypair(ctx context.Context, projectID, name string) (Keypair, error)

	// DeleteKeypair removes the keypair. Idempotent — not-found is not an error.
	DeleteKeypair(ctx context.Context, projectID, name string) error

	// BootServer launches a VM from a Glance image into the project.
	// Returns the server ID immediately; use WaitForActive for readiness.
	BootServer(ctx context.Context, projectID string, spec ServerSpec) (string, error)

	// WaitForActive polls until the server is ACTIVE or timeout/ctx cancellation.
	WaitForActive(ctx context.Context, projectID, serverID string) error

	// AllocateFloatingIP allocates a floating IP and associates it with the server.
	AllocateFloatingIP(ctx context.Context, projectID, serverID string) (string, error)

	// DeleteServer stops and deletes the server. Idempotent.
	DeleteServer(ctx context.Context, projectID, serverID string) error

	// CreateNetwork creates an isolated tenant network with the given CIDR
	// and connects it to the public network via a router.
	CreateNetwork(ctx context.Context, projectID string, spec NetworkSpec) (string, error)

	// DeleteNetwork removes the network and all router interfaces. Idempotent.
	DeleteNetwork(ctx context.Context, projectID, networkID string) error
}

// Keypair carries the generated SSH key material.
type Keypair struct {
	Name        string
	PublicKey   string
	PrivateKey  []byte // PEM, sensitive — caller MUST encrypt before storage
	Fingerprint string
}

// ServerSpec describes a VM to boot. Maps to OpenStack server-create.
type ServerSpec struct {
	Name        string
	ImageRef    string // Glance image name or UUID
	FlavorRef   string
	NetworkID   string
	KeypairName string
	UserData    string // cloud-init
	Metadata    map[string]string
}

// NetworkSpec describes an isolated tenant network.
type NetworkSpec struct {
	Name string
	CIDR string
}
