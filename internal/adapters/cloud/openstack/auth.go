package openstack

import "github.com/gophercloud/gophercloud/v2"

func (p *Provider) authOptions(projectID string) gophercloud.AuthOptions {
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: p.cfg.AuthURL,
		Username:         p.cfg.Username,
		Password:         p.cfg.Password,
		DomainName:       p.cfg.DomainName,
		AllowReauth:      true,
	}
	if projectID != "" {
		opts.TenantID = projectID
		return opts
	}
	opts.TenantName = p.cfg.ProjectName
	return opts
}
