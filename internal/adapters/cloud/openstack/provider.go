// Package openstack implements ports.CloudProvider with gophercloud/v2.
package openstack

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/cloud-lab-gateway/gateway/pkg/config"
	"github.com/gophercloud/gophercloud/v2"
	osclient "github.com/gophercloud/gophercloud/v2/openstack"
)

const adminProjectKey = "_admin"

// Provider is the production КИ/OpenStack CloudProvider adapter.
type Provider struct {
	cfg          config.OpenStack
	waitInterval time.Duration

	mu      sync.RWMutex
	clients map[string]*gophercloud.ProviderClient
}

var _ ports.CloudProvider = (*Provider)(nil)

// New creates an OpenStack provider. Authentication is lazy and cached per
// project scope, so construction does not hit Keystone.
func New(cfg config.OpenStack) (*Provider, error) {
	if cfg.AuthURL == "" {
		return nil, fmt.Errorf("openstack: auth url is required")
	}
	if cfg.Username == "" || cfg.Password == "" {
		return nil, fmt.Errorf("openstack: username and password are required")
	}
	if cfg.Region == "" {
		cfg.Region = "RegionOne"
	}
	return &Provider{
		cfg:          cfg,
		waitInterval: 5 * time.Second,
		clients:      map[string]*gophercloud.ProviderClient{},
	}, nil
}

func (p *Provider) endpointOpts() gophercloud.EndpointOpts {
	return gophercloud.EndpointOpts{Region: p.cfg.Region}
}

func (p *Provider) compute(ctx context.Context, projectID string) (*gophercloud.ServiceClient, error) {
	provider, err := p.provider(ctx, projectID)
	if err != nil {
		return nil, err
	}
	client, err := osclient.NewComputeV2(provider, p.endpointOpts())
	if err != nil {
		return nil, translateErr("compute client", err)
	}
	return client, nil
}

func (p *Provider) network(ctx context.Context, projectID string) (*gophercloud.ServiceClient, error) {
	provider, err := p.provider(ctx, projectID)
	if err != nil {
		return nil, err
	}
	client, err := osclient.NewNetworkV2(provider, p.endpointOpts())
	if err != nil {
		return nil, translateErr("network client", err)
	}
	return client, nil
}

func (p *Provider) blockStorage(ctx context.Context, projectID string) (*gophercloud.ServiceClient, error) {
	provider, err := p.provider(ctx, projectID)
	if err != nil {
		return nil, err
	}
	client, err := osclient.NewBlockStorageV3(provider, p.endpointOpts())
	if err != nil {
		return nil, translateErr("block storage client", err)
	}
	return client, nil
}

func (p *Provider) image(ctx context.Context, projectID string) (*gophercloud.ServiceClient, error) {
	provider, err := p.provider(ctx, projectID)
	if err != nil {
		return nil, err
	}
	client, err := osclient.NewImageV2(provider, p.endpointOpts())
	if err != nil {
		return nil, translateErr("image client", err)
	}
	return client, nil
}

func (p *Provider) provider(ctx context.Context, projectID string) (*gophercloud.ProviderClient, error) {
	key := projectID
	if key == "" {
		key = adminProjectKey
	}

	p.mu.RLock()
	cached := p.clients[key]
	p.mu.RUnlock()
	if cached != nil {
		return cached, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if cached = p.clients[key]; cached != nil {
		return cached, nil
	}

	provider, err := osclient.NewClient(p.cfg.AuthURL)
	if err != nil {
		return nil, translateErr("create provider client", err)
	}
	if !p.cfg.VerifyTLS {
		// #nosec G402 -- Hackathon/private КИ contours may use private CA; env controls this explicitly.
		provider.HTTPClient = http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	}
	provider.UserAgent.Prepend("cloud-lab-gateway")
	if err := osclient.Authenticate(ctx, provider, p.authOptions(projectID)); err != nil {
		return nil, translateErr("authenticate", err)
	}
	p.clients[key] = provider
	return provider, nil
}
