// Package inmem implements ports.CloudProvider against an in-memory state
// machine. It exists for two reasons:
//
//  1. Local development without КИ access. The team can build end-to-end
//     scenarios before the real cluster is reachable.
//  2. Deterministic integration tests. Every operation is fast and side-effect
//     free; failures can be injected via the Faults knob.
//
// Behaviour mirrors gophercloud's adapter as closely as practical:
//   - all operations are idempotent
//   - resources are scoped per project
//   - BootServer returns immediately; WaitForActive simulates boot latency
package inmem

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

// Capacity describes the simulated cluster.
type Capacity struct {
	VCPUs  int64
	RAMMB  int64
	DiskGB int64
}

// DefaultCapacity returns a reasonable default cluster size.
func DefaultCapacity() Capacity {
	return Capacity{VCPUs: 100, RAMMB: 256 * 1024, DiskGB: 4 * 1024}
}

// Faults injects errors for testing. Each method consulted before doing work.
type Faults struct {
	GetQuotaSnapshotErr   error
	CreateKeypairErr      error
	DeleteKeypairErr      error
	BootServerErr         error
	WaitForActiveErr      error
	AllocateFloatingIPErr error
	DeleteServerErr       error
	CreateNetworkErr      error
	DeleteNetworkErr      error

	// BootLatency controls how long WaitForActive takes; tests usually use 0.
	BootLatency time.Duration
}

// Provider is the in-memory implementation of ports.CloudProvider.
type Provider struct {
	mu       sync.Mutex
	capacity Capacity
	used     Capacity
	servers  map[string]*server  // serverID → server
	keypairs map[string]*keypair // projectID+name → keypair
	networks map[string]*network // networkID → network
	floating map[string]string   // ip → serverID
	faults   Faults
	now      func() time.Time

	// Counter for synthesised IDs.
	seq int64
}

type server struct {
	id          string
	projectID   string
	name        string
	imageRef    string
	flavorRef   string
	networkID   string
	keypairName string
	metadata    map[string]string
	bootedAt    time.Time
	resources   shared.ResourceRequest
}

type keypair struct {
	name        string
	projectID   string
	publicKey   string
	privateKey  []byte
	fingerprint string
}

type network struct {
	id        string
	projectID string
	name      string
	cidr      string
}

// New creates a fresh in-memory provider.
func New(capacity Capacity, faults Faults) *Provider {
	if capacity == (Capacity{}) {
		capacity = DefaultCapacity()
	}
	return &Provider{
		capacity: capacity,
		servers:  map[string]*server{},
		keypairs: map[string]*keypair{},
		networks: map[string]*network{},
		floating: map[string]string{},
		faults:   faults,
		now:      time.Now,
	}
}

// SetClock replaces the time source for tests.
func (p *Provider) SetClock(fn func() time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.now = fn
}

// Compile-time check.
var _ ports.CloudProvider = (*Provider)(nil)

// GetQuotaSnapshot returns simulated cluster utilisation.
func (p *Provider) GetQuotaSnapshot(_ context.Context) (shared.QuotaSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.faults.GetQuotaSnapshotErr != nil {
		return shared.QuotaSnapshot{}, p.faults.GetQuotaSnapshotErr
	}
	return shared.QuotaSnapshot{
		VCPUs:     shared.Capacity{Used: p.used.VCPUs, Total: p.capacity.VCPUs, Unit: "vcpu"},
		RAM:       shared.Capacity{Used: p.used.RAMMB, Total: p.capacity.RAMMB, Unit: "MB"},
		Disk:      shared.Capacity{Used: p.used.DiskGB, Total: p.capacity.DiskGB, Unit: "GB"},
		FetchedAt: p.now(),
	}, nil
}

// CreateKeypair generates an ed25519 keypair and stores it. Idempotent on (project, name).
func (p *Provider) CreateKeypair(_ context.Context, projectID, name string) (ports.Keypair, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.faults.CreateKeypairErr != nil {
		return ports.Keypair{}, p.faults.CreateKeypairErr
	}
	key := projectID + "/" + name
	if existing, ok := p.keypairs[key]; ok {
		return ports.Keypair{
			Name:        existing.name,
			PublicKey:   existing.publicKey,
			PrivateKey:  append([]byte(nil), existing.privateKey...),
			Fingerprint: existing.fingerprint,
		}, nil
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return ports.Keypair{}, fmt.Errorf("generate keypair: %w", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "OPENSSH PRIVATE KEY", Bytes: priv})
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	kp := &keypair{
		name:        name,
		projectID:   projectID,
		publicKey:   "ssh-ed25519 " + pubB64,
		privateKey:  privPEM,
		fingerprint: fmt.Sprintf("SHA256:%x", pub[:16]),
	}
	p.keypairs[key] = kp
	return ports.Keypair{
		Name:        kp.name,
		PublicKey:   kp.publicKey,
		PrivateKey:  append([]byte(nil), kp.privateKey...),
		Fingerprint: kp.fingerprint,
	}, nil
}

// DeleteKeypair removes a keypair. Not-found is not an error.
func (p *Provider) DeleteKeypair(_ context.Context, projectID, name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.faults.DeleteKeypairErr != nil {
		return p.faults.DeleteKeypairErr
	}
	delete(p.keypairs, projectID+"/"+name)
	return nil
}

// BootServer allocates resources and returns immediately with a server ID.
func (p *Provider) BootServer(_ context.Context, projectID string, spec ports.ServerSpec) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.faults.BootServerErr != nil {
		return "", p.faults.BootServerErr
	}
	// Idempotency: re-use a server with same project + name if present.
	for id, s := range p.servers {
		if s.projectID == projectID && s.name == spec.Name {
			return id, nil
		}
	}
	// Resource accounting (rough heuristic).
	req := requestFromFlavor(spec.FlavorRef)
	if !fits(p.used, req, p.capacity) {
		return "", errors.New("inmem: cluster capacity exhausted")
	}
	id := p.nextID("srv")
	p.servers[id] = &server{
		id:          id,
		projectID:   projectID,
		name:        spec.Name,
		imageRef:    spec.ImageRef,
		flavorRef:   spec.FlavorRef,
		networkID:   spec.NetworkID,
		keypairName: spec.KeypairName,
		metadata:    copyMeta(spec.Metadata),
		bootedAt:    p.now().Add(p.faults.BootLatency),
		resources:   req,
	}
	p.used.VCPUs += int64(req.VCPUs)
	p.used.RAMMB += int64(req.RAMMB)
	p.used.DiskGB += int64(req.DiskGB)
	return id, nil
}

// WaitForActive blocks until the simulated boot time has passed.
func (p *Provider) WaitForActive(ctx context.Context, _ string, serverID string) error {
	p.mu.Lock()
	if p.faults.WaitForActiveErr != nil {
		err := p.faults.WaitForActiveErr
		p.mu.Unlock()
		return err
	}
	s, ok := p.servers[serverID]
	if !ok {
		p.mu.Unlock()
		return errors.New("inmem: server not found")
	}
	wait := time.Until(s.bootedAt)
	p.mu.Unlock()

	if wait <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}

// AllocateFloatingIP synthesises a deterministic IP and associates it.
func (p *Provider) AllocateFloatingIP(_ context.Context, _ string, serverID string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.faults.AllocateFloatingIPErr != nil {
		return "", p.faults.AllocateFloatingIPErr
	}
	if _, ok := p.servers[serverID]; !ok {
		return "", errors.New("inmem: server not found")
	}
	// Look for an existing FIP for this server (idempotency).
	for ip, sid := range p.floating {
		if sid == serverID {
			return ip, nil
		}
	}
	p.seq++
	ip := fmt.Sprintf("203.0.113.%d", (p.seq%253)+1)
	p.floating[ip] = serverID
	return ip, nil
}

// DeleteServer removes the server and frees its resources. Idempotent.
func (p *Provider) DeleteServer(_ context.Context, _ string, serverID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.faults.DeleteServerErr != nil {
		return p.faults.DeleteServerErr
	}
	s, ok := p.servers[serverID]
	if !ok {
		return nil
	}
	p.used.VCPUs -= int64(s.resources.VCPUs)
	p.used.RAMMB -= int64(s.resources.RAMMB)
	p.used.DiskGB -= int64(s.resources.DiskGB)
	delete(p.servers, serverID)
	for ip, sid := range p.floating {
		if sid == serverID {
			delete(p.floating, ip)
		}
	}
	return nil
}

// CreateNetwork creates an isolated network. Idempotent on (project, name).
func (p *Provider) CreateNetwork(_ context.Context, projectID string, spec ports.NetworkSpec) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.faults.CreateNetworkErr != nil {
		return "", p.faults.CreateNetworkErr
	}
	for id, n := range p.networks {
		if n.projectID == projectID && n.name == spec.Name {
			return id, nil
		}
	}
	id := p.nextID("net")
	p.networks[id] = &network{id: id, projectID: projectID, name: spec.Name, cidr: spec.CIDR}
	return id, nil
}

// DeleteNetwork removes a network. Idempotent.
func (p *Provider) DeleteNetwork(_ context.Context, _ string, networkID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.faults.DeleteNetworkErr != nil {
		return p.faults.DeleteNetworkErr
	}
	delete(p.networks, networkID)
	return nil
}

// --- test helpers -----------------------------------------------------------

// Snapshot returns a read-only view of internal state (for tests).
type Snapshot struct {
	UsedVCPUs  int64
	UsedRAMMB  int64
	UsedDiskGB int64
	Servers    []string
	Networks   []string
	Keypairs   []string
	Floating   map[string]string
}

func (p *Provider) Snapshot() Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	srvIDs := make([]string, 0, len(p.servers))
	for id := range p.servers {
		srvIDs = append(srvIDs, id)
	}
	sort.Strings(srvIDs)
	netIDs := make([]string, 0, len(p.networks))
	for id := range p.networks {
		netIDs = append(netIDs, id)
	}
	sort.Strings(netIDs)
	keyIDs := make([]string, 0, len(p.keypairs))
	for k := range p.keypairs {
		keyIDs = append(keyIDs, k)
	}
	sort.Strings(keyIDs)
	fips := make(map[string]string, len(p.floating))
	for k, v := range p.floating {
		fips[k] = v
	}
	return Snapshot{
		UsedVCPUs:  p.used.VCPUs,
		UsedRAMMB:  p.used.RAMMB,
		UsedDiskGB: p.used.DiskGB,
		Servers:    srvIDs,
		Networks:   netIDs,
		Keypairs:   keyIDs,
		Floating:   fips,
	}
}

func (p *Provider) nextID(prefix string) string {
	p.seq++
	return fmt.Sprintf("%s-%06d", prefix, p.seq)
}

// requestFromFlavor maps common OpenStack (m1.*) and КИ (start/small/medium/
// large) flavor names to a ResourceRequest. Unknown flavors get a
// conservative default.
func requestFromFlavor(flavor string) shared.ResourceRequest {
	switch flavor {
	case "m1.tiny":
		return shared.ResourceRequest{VCPUs: 1, RAMMB: 512, DiskGB: 1}
	case "start": // КИ: 1 ЦП / 1 ГБ
		return shared.ResourceRequest{VCPUs: 1, RAMMB: 1024, DiskGB: 10}
	case "m1.small", "small": // КИ small: 1 ЦП / 2 ГБ
		return shared.ResourceRequest{VCPUs: 1, RAMMB: 2048, DiskGB: 20}
	case "m1.medium", "medium": // КИ medium: 2 ЦП / 4 ГБ
		return shared.ResourceRequest{VCPUs: 2, RAMMB: 4096, DiskGB: 40}
	case "m1.large", "large": // КИ large: 4 ЦП / 8 ГБ
		return shared.ResourceRequest{VCPUs: 4, RAMMB: 8192, DiskGB: 80}
	default:
		return shared.ResourceRequest{VCPUs: 1, RAMMB: 2048, DiskGB: 20}
	}
}

func fits(used Capacity, req shared.ResourceRequest, capacity Capacity) bool {
	return used.VCPUs+int64(req.VCPUs) <= capacity.VCPUs &&
		used.RAMMB+int64(req.RAMMB) <= capacity.RAMMB &&
		used.DiskGB+int64(req.DiskGB) <= capacity.DiskGB
}

func copyMeta(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
