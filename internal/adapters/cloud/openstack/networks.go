package openstack

import (
	"context"

	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/layer3/routers"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/networks"
	networkports "github.com/gophercloud/gophercloud/v2/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/subnets"
)

// CreateNetwork creates an isolated tenant network, subnet, and optional router.
func (p *Provider) CreateNetwork(ctx context.Context, projectID string, spec ports.NetworkSpec) (string, error) {
	networkClient, err := p.network(ctx, projectID)
	if err != nil {
		return "", err
	}

	existing, err := p.findNetworkByName(ctx, networkClient, projectID, spec.Name)
	if err != nil {
		return "", err
	}
	if existing != nil {
		return existing.ID, nil
	}

	up := true
	created, err := networks.Create(ctx, networkClient, networks.CreateOpts{
		Name:         spec.Name,
		AdminStateUp: &up,
		ProjectID:    projectID,
		TenantID:     projectID,
	}).Extract()
	if err != nil {
		return "", translateErr("create network", err)
	}

	subnet, err := p.ensureSubnet(ctx, networkClient, projectID, created.ID, spec)
	if err != nil {
		return "", err
	}
	if p.cfg.PublicNetworkID != "" {
		if err := p.ensureRouterInterface(ctx, networkClient, projectID, spec.Name, subnet.ID); err != nil {
			return "", err
		}
	}
	return created.ID, nil
}

// DeleteNetwork removes router interfaces, subnets, and the network. Missing
// resources are ignored so cleanup saga retries remain idempotent.
func (p *Provider) DeleteNetwork(ctx context.Context, projectID, networkID string) error {
	networkClient, err := p.network(ctx, projectID)
	if err != nil {
		return err
	}

	portPage, err := networkports.List(networkClient, networkports.ListOpts{
		NetworkID:   networkID,
		ProjectID:   projectID,
		DeviceOwner: "network:router_interface",
	}).AllPages(ctx)
	if err != nil && !isNotFound(err) {
		return translateErr("list router ports", err)
	}
	if err == nil {
		routerPorts, err := networkports.ExtractPorts(portPage)
		if err != nil {
			return translateErr("extract router ports", err)
		}
		for _, port := range routerPorts {
			for _, fixedIP := range port.FixedIPs {
				_, removeErr := routers.RemoveInterface(ctx, networkClient, port.DeviceID, routers.RemoveInterfaceOpts{SubnetID: fixedIP.SubnetID}).Extract()
				if removeErr := ignoreNotFound(translateErr("remove router interface", removeErr)); removeErr != nil && !isConflict(removeErr) {
					return removeErr
				}
			}
			if port.DeviceID != "" {
				deleteErr := routers.Delete(ctx, networkClient, port.DeviceID).ExtractErr()
				if deleteErr := ignoreNotFound(translateErr("delete router", deleteErr)); deleteErr != nil && !isConflict(deleteErr) {
					return deleteErr
				}
			}
		}
	}

	subnetPage, err := subnets.List(networkClient, subnets.ListOpts{NetworkID: networkID, ProjectID: projectID}).AllPages(ctx)
	if err != nil && !isNotFound(err) {
		return translateErr("list subnets", err)
	}
	if err == nil {
		allSubnets, err := subnets.ExtractSubnets(subnetPage)
		if err != nil {
			return translateErr("extract subnets", err)
		}
		for _, subnet := range allSubnets {
			deleteErr := subnets.Delete(ctx, networkClient, subnet.ID).ExtractErr()
			if deleteErr := ignoreNotFound(translateErr("delete subnet", deleteErr)); deleteErr != nil && !isConflict(deleteErr) {
				return deleteErr
			}
		}
	}

	err = networks.Delete(ctx, networkClient, networkID).ExtractErr()
	return ignoreNotFound(translateErr("delete network", err))
}

func (p *Provider) findNetworkByName(ctx context.Context, client *gophercloud.ServiceClient, projectID, name string) (*networks.Network, error) {
	page, err := networks.List(client, networks.ListOpts{Name: name, ProjectID: projectID}).AllPages(ctx)
	if err != nil {
		return nil, translateErr("list networks", err)
	}
	all, err := networks.ExtractNetworks(page)
	if err != nil {
		return nil, translateErr("extract networks", err)
	}
	for i := range all {
		if all[i].Name == name {
			return &all[i], nil
		}
	}
	return nil, nil
}

func (p *Provider) ensureSubnet(ctx context.Context, client *gophercloud.ServiceClient, projectID, networkID string, spec ports.NetworkSpec) (*subnets.Subnet, error) {
	name := spec.Name + "-subnet"
	page, err := subnets.List(client, subnets.ListOpts{Name: name, NetworkID: networkID, ProjectID: projectID}).AllPages(ctx)
	if err != nil {
		return nil, translateErr("list subnets", err)
	}
	all, err := subnets.ExtractSubnets(page)
	if err != nil {
		return nil, translateErr("extract subnets", err)
	}
	for i := range all {
		if all[i].Name == name {
			return &all[i], nil
		}
	}

	enableDHCP := true
	created, err := subnets.Create(ctx, client, subnets.CreateOpts{
		NetworkID:  networkID,
		CIDR:       spec.CIDR,
		Name:       name,
		IPVersion:  gophercloud.IPv4,
		EnableDHCP: &enableDHCP,
		ProjectID:  projectID,
		TenantID:   projectID,
	}).Extract()
	if err != nil {
		return nil, translateErr("create subnet", err)
	}
	return created, nil
}

func (p *Provider) ensureRouterInterface(ctx context.Context, client *gophercloud.ServiceClient, projectID, networkName, subnetID string) error {
	routerName := networkName + "-router"
	page, err := routers.List(client, routers.ListOpts{Name: routerName, ProjectID: projectID}).AllPages(ctx)
	if err != nil {
		return translateErr("list routers", err)
	}
	all, err := routers.ExtractRouters(page)
	if err != nil {
		return translateErr("extract routers", err)
	}
	var routerID string
	for _, router := range all {
		if router.Name == routerName {
			routerID = router.ID
			break
		}
	}
	if routerID == "" {
		snat := true
		router, err := routers.Create(ctx, client, routers.CreateOpts{
			Name:      routerName,
			ProjectID: projectID,
			TenantID:  projectID,
			GatewayInfo: &routers.GatewayInfo{
				NetworkID:  p.cfg.PublicNetworkID,
				EnableSNAT: &snat,
			},
		}).Extract()
		if err != nil {
			return translateErr("create router", err)
		}
		routerID = router.ID
	}

	_, err = routers.AddInterface(ctx, client, routerID, routers.AddInterfaceOpts{SubnetID: subnetID}).Extract()
	if err != nil && !isConflict(err) {
		return translateErr("add router interface", err)
	}
	return nil
}
