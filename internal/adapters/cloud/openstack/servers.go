package openstack

import (
	"context"
	"fmt"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/flavors"
	computekeypairs "github.com/gophercloud/gophercloud/v2/openstack/compute/v2/keypairs"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/images"
)

// BootServer launches a VM and returns immediately with its server ID.
func (p *Provider) BootServer(ctx context.Context, projectID string, spec ports.ServerSpec) (string, error) {
	compute, err := p.compute(ctx, projectID)
	if err != nil {
		return "", err
	}

	existing, err := p.findServerByName(ctx, compute, spec.Name)
	if err != nil {
		return "", err
	}
	if existing != nil {
		return existing.ID, nil
	}

	imageRef, err := p.resolveImageRef(ctx, projectID, spec.ImageRef)
	if err != nil {
		return "", err
	}
	flavorRef, err := p.resolveFlavorRef(ctx, compute, spec.FlavorRef)
	if err != nil {
		return "", err
	}

	createOpts := servers.CreateOpts{
		Name:      spec.Name,
		ImageRef:  imageRef,
		FlavorRef: flavorRef,
		UserData:  []byte(spec.UserData),
		Metadata:  spec.Metadata,
	}
	if spec.NetworkID != "" {
		createOpts.Networks = []servers.Network{{UUID: spec.NetworkID}}
	}
	var createBuilder servers.CreateOptsBuilder = createOpts
	if spec.KeypairName != "" {
		createBuilder = computekeypairs.CreateOptsExt{
			CreateOptsBuilder: createOpts,
			KeyName:           spec.KeypairName,
		}
	}

	created, err := servers.Create(ctx, compute, createBuilder, nil).Extract()
	if err != nil {
		return "", translateErr("boot server", err)
	}
	return created.ID, nil
}

// WaitForActive polls Nova until the server is ACTIVE or the context ends.
func (p *Provider) WaitForActive(ctx context.Context, projectID, serverID string) error {
	compute, err := p.compute(ctx, projectID)
	if err != nil {
		return err
	}

	for {
		server, err := servers.Get(ctx, compute, serverID).Extract()
		if err != nil {
			return translateErr("get server status", err)
		}
		switch server.Status {
		case "ACTIVE":
			return nil
		case "ERROR":
			return fmt.Errorf("openstack: server %s entered ERROR state", serverID)
		}

		timer := time.NewTimer(p.waitInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// DeleteServer deletes a VM and its associated floating IPs. Missing servers are ignored.
func (p *Provider) DeleteServer(ctx context.Context, projectID, serverID string) error {
	compute, err := p.compute(ctx, projectID)
	if err != nil {
		return err
	}
	_ = p.releaseFloatingIPsForServer(ctx, projectID, serverID)
	err = servers.Delete(ctx, compute, serverID).ExtractErr()
	return ignoreNotFound(translateErr("delete server", err))
}

func (p *Provider) findServerByName(ctx context.Context, client *gophercloud.ServiceClient, name string) (*servers.Server, error) {
	opts := servers.ListOpts{Name: "^" + name + "$"}
	page, err := servers.List(client, opts).AllPages(ctx)
	if err != nil {
		return nil, translateErr("list servers", err)
	}
	all, err := servers.ExtractServers(page)
	if err != nil {
		return nil, translateErr("extract servers", err)
	}
	for i := range all {
		if all[i].Name == name {
			return &all[i], nil
		}
	}
	return nil, nil
}

func (p *Provider) resolveImageRef(ctx context.Context, projectID, ref string) (string, error) {
	if _, err := uuid.Parse(ref); err == nil {
		return ref, nil
	}
	imageClient, err := p.image(ctx, projectID)
	if err != nil {
		return "", err
	}
	page, err := images.List(imageClient, images.ListOpts{Name: ref, Limit: 2}).AllPages(ctx)
	if err != nil {
		return "", translateErr("list images", err)
	}
	all, err := images.ExtractImages(page)
	if err != nil {
		return "", translateErr("extract images", err)
	}
	for _, image := range all {
		if image.Name == ref {
			return image.ID, nil
		}
	}
	return "", fmt.Errorf("openstack: image %q: %w", ref, shared.ErrNotFound)
}

func (p *Provider) resolveFlavorRef(ctx context.Context, compute *gophercloud.ServiceClient, ref string) (string, error) {
	if _, err := uuid.Parse(ref); err == nil {
		return ref, nil
	}
	page, err := flavors.ListDetail(compute, nil).AllPages(ctx)
	if err != nil {
		return "", translateErr("list flavors", err)
	}
	all, err := flavors.ExtractFlavors(page)
	if err != nil {
		return "", translateErr("extract flavors", err)
	}
	for _, flavor := range all {
		if flavor.Name == ref || flavor.ID == ref {
			return flavor.ID, nil
		}
	}
	return "", fmt.Errorf("openstack: flavor %q: %w", ref, shared.ErrNotFound)
}
