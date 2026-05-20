package openstack

import (
	"context"
	"fmt"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/layer3/floatingips"
	networkports "github.com/gophercloud/gophercloud/v2/openstack/networking/v2/ports"
)

// AllocateFloatingIP allocates and associates a floating IP with the server.
func (p *Provider) AllocateFloatingIP(ctx context.Context, projectID, serverID string) (string, error) {
	network, err := p.network(ctx, projectID)
	if err != nil {
		return "", err
	}
	if p.cfg.PublicNetworkID == "" {
		return "", fmt.Errorf("openstack: public network id is required for floating IP allocation")
	}

	port, err := p.firstServerPort(ctx, projectID, serverID)
	if err != nil {
		return "", err
	}
	existing, err := floatingips.List(network, floatingips.ListOpts{PortID: port.ID, ProjectID: projectID}).AllPages(ctx)
	if err != nil {
		return "", translateErr("list floating IPs", err)
	}
	all, err := floatingips.ExtractFloatingIPs(existing)
	if err != nil {
		return "", translateErr("extract floating IPs", err)
	}
	for _, fip := range all {
		if fip.PortID == port.ID {
			return fip.FloatingIP, nil
		}
	}

	created, err := floatingips.Create(ctx, network, floatingips.CreateOpts{
		FloatingNetworkID: p.cfg.PublicNetworkID,
		PortID:            port.ID,
		ProjectID:         projectID,
		TenantID:          projectID,
	}).Extract()
	if err != nil {
		return "", translateErr("create floating IP", err)
	}
	return created.FloatingIP, nil
}

func (p *Provider) releaseFloatingIPsForServer(ctx context.Context, projectID, serverID string) error {
	network, err := p.network(ctx, projectID)
	if err != nil {
		return err
	}
	page, err := networkports.List(network, networkports.ListOpts{
		DeviceID:  serverID,
		ProjectID: projectID,
	}).AllPages(ctx)
	if err != nil {
		return translateErr("list server ports", err)
	}
	serverPorts, err := networkports.ExtractPorts(page)
	if err != nil {
		return translateErr("extract server ports", err)
	}
	for _, port := range serverPorts {
		fipPage, err := floatingips.List(network, floatingips.ListOpts{PortID: port.ID, ProjectID: projectID}).AllPages(ctx)
		if err != nil {
			return translateErr("list server floating IPs", err)
		}
		fips, err := floatingips.ExtractFloatingIPs(fipPage)
		if err != nil {
			return translateErr("extract server floating IPs", err)
		}
		for _, fip := range fips {
			err := floatingips.Delete(ctx, network, fip.ID).ExtractErr()
			if err := ignoreNotFound(translateErr("delete floating IP", err)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *Provider) firstServerPort(ctx context.Context, projectID, serverID string) (*networkports.Port, error) {
	network, err := p.network(ctx, projectID)
	if err != nil {
		return nil, err
	}
	page, err := networkports.List(network, networkports.ListOpts{
		DeviceID:  serverID,
		ProjectID: projectID,
	}).AllPages(ctx)
	if err != nil {
		return nil, translateErr("list server ports", err)
	}
	all, err := networkports.ExtractPorts(page)
	if err != nil {
		return nil, translateErr("extract server ports", err)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("openstack: server %s port: %w", serverID, shared.ErrNotFound)
	}
	return &all[0], nil
}
