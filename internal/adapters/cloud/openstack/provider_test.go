package openstack

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/cloud-lab-gateway/gateway/pkg/config"
)

func TestProviderImplementsCloudProvider(t *testing.T) {
	t.Parallel()
	var _ ports.CloudProvider = (*Provider)(nil)
}

func TestSmokeSkippedWithoutOpenStackEnv(t *testing.T) {
	if os.Getenv("OPENSTACK_AUTH_URL") == "" || os.Getenv("OPENSTACK_PASSWORD") == "" {
		t.Skip("OPENSTACK_AUTH_URL/OPENSTACK_PASSWORD are not set")
	}

	p, err := New(config.OpenStack{
		AuthURL:       os.Getenv("OPENSTACK_AUTH_URL"),
		Username:      os.Getenv("OPENSTACK_USERNAME"),
		Password:      os.Getenv("OPENSTACK_PASSWORD"),
		DomainName:    getenv("OPENSTACK_DOMAIN_NAME", "Default"),
		ProjectName:   os.Getenv("OPENSTACK_PROJECT_NAME"),
		Region:        getenv("OPENSTACK_REGION", "RegionOne"),
		VerifyTLS:     true,
		DefaultFlavor: getenv("OPENSTACK_DEFAULT_FLAVOR", "m1.tiny"),
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := p.GetQuotaSnapshot(ctx); err != nil {
		t.Fatalf("quota snapshot: %v", err)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
