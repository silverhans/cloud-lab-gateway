package openstack

import (
	"context"

	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/keypairs"
)

// CreateKeypair creates a project-scoped SSH keypair. If the key already
// exists, OpenStack only returns public material; PrivateKey is nil in that path.
func (p *Provider) CreateKeypair(ctx context.Context, projectID, name string) (ports.Keypair, error) {
	compute, err := p.compute(ctx, projectID)
	if err != nil {
		return ports.Keypair{}, err
	}

	existing, err := keypairs.Get(ctx, compute, name, nil).Extract()
	if err == nil && existing != nil {
		return keypairFromOpenStack(*existing), nil
	}
	if err != nil && !isNotFound(err) {
		return ports.Keypair{}, translateErr("get keypair", err)
	}

	created, err := keypairs.Create(ctx, compute, keypairs.CreateOpts{Name: name, Type: "ssh"}).Extract()
	if err != nil {
		return ports.Keypair{}, translateErr("create keypair", err)
	}
	return keypairFromOpenStack(*created), nil
}

// DeleteKeypair deletes a project-scoped keypair. Missing keypairs are ignored.
func (p *Provider) DeleteKeypair(ctx context.Context, projectID, name string) error {
	compute, err := p.compute(ctx, projectID)
	if err != nil {
		return err
	}
	err = keypairs.Delete(ctx, compute, name, nil).ExtractErr()
	return ignoreNotFound(translateErr("delete keypair", err))
}

func keypairFromOpenStack(k keypairs.KeyPair) ports.Keypair {
	return ports.Keypair{
		Name:        k.Name,
		PublicKey:   k.PublicKey,
		PrivateKey:  []byte(k.PrivateKey),
		Fingerprint: k.Fingerprint,
	}
}
