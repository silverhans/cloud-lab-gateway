// Package config loads runtime configuration from environment variables
// (12-factor). Values come from .env (via godotenv if present) or the process
// environment. Secrets are NEVER read from files committed to the repo.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config is the root configuration struct shared by gateway and worker.
type Config struct {
	BindAddr      string
	LogLevel      string
	CloudProvider string
	PG            PG
	Redis         Redis
	KEK           KEK
	JWT           JWT
	Quota         Quota
	Lifecycle     Lifecycle
	OpenStack     OpenStack
}

type PG struct {
	DSN string
}

type Redis struct {
	Addr string
}

type KEK struct {
	Base64  string
	Version int
}

type JWT struct {
	Secret string
	TTL    time.Duration
}

type Quota struct {
	ThresholdPct    float64
	CacheTTLSeconds int
}

type Lifecycle struct {
	DefaultCleanup time.Duration
	DefaultFreeze  time.Duration
	DeployTimeout  time.Duration
	CheckTimeout   time.Duration
}

type OpenStack struct {
	AuthURL         string
	Username        string
	Password        string
	DomainName      string
	ProjectName     string
	Region          string
	VerifyTLS       bool
	DefaultFlavor   string
	PublicNetworkID string
}

// Load reads config from env (.env is loaded if present). Returns an error if
// required values are missing.
func Load() (Config, error) {
	_ = godotenv.Load()
	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	cfg := Config{
		BindAddr:      getString(v, "CLG_BIND_ADDR", "0.0.0.0:8080"),
		LogLevel:      getString(v, "LOG_LEVEL", "info"),
		CloudProvider: getString(v, "CLG_CLOUD_PROVIDER", "inmem"),
		PG: PG{
			DSN: getString(v, "PG_DSN", ""),
		},
		Redis: Redis{
			Addr: getString(v, "REDIS_ADDR", "redis:6379"),
		},
		KEK: KEK{
			Base64:  getString(v, "CLG_KEK_BASE64", ""),
			Version: getInt(v, "CLG_KEK_VERSION", 1),
		},
		JWT: JWT{
			Secret: getString(v, "CLG_JWT_SECRET", ""),
			TTL:    time.Duration(getInt(v, "CLG_JWT_TTL_SECONDS", 28800)) * time.Second,
		},
		Quota: Quota{
			ThresholdPct:    getFloat(v, "CLG_QUOTA_THRESHOLD_PCT", 90.0),
			CacheTTLSeconds: getInt(v, "CLG_QUOTA_CACHE_TTL_SECONDS", 30),
		},
		Lifecycle: Lifecycle{
			DefaultCleanup: time.Duration(getInt(v, "CLG_DEFAULT_CLEANUP_SECONDS", 7200)) * time.Second,
			DefaultFreeze:  time.Duration(getInt(v, "CLG_DEFAULT_FREEZE_SECONDS", 86400)) * time.Second,
			DeployTimeout:  time.Duration(getInt(v, "CLG_DEPLOY_TIMEOUT_SECONDS", 600)) * time.Second,
			CheckTimeout:   time.Duration(getInt(v, "CLG_CHECK_TIMEOUT_SECONDS", 300)) * time.Second,
		},
		OpenStack: OpenStack{
			AuthURL:         getString(v, "OPENSTACK_AUTH_URL", ""),
			Username:        getString(v, "OPENSTACK_USERNAME", ""),
			Password:        getString(v, "OPENSTACK_PASSWORD", ""),
			DomainName:      getString(v, "OPENSTACK_DOMAIN_NAME", "Default"),
			ProjectName:     getString(v, "OPENSTACK_PROJECT_NAME", ""),
			Region:          getString(v, "OPENSTACK_REGION", "RegionOne"),
			VerifyTLS:       getBool(v, "OPENSTACK_VERIFY_TLS", true),
			DefaultFlavor:   getString(v, "OPENSTACK_DEFAULT_FLAVOR", "m1.small"),
			PublicNetworkID: getString(v, "OPENSTACK_PUBLIC_NETWORK_ID", ""),
		},
	}

	return cfg, cfg.validate()
}

func (c Config) validate() error {
	var missing []string
	if c.PG.DSN == "" {
		missing = append(missing, "PG_DSN")
	}
	if c.KEK.Base64 == "" {
		missing = append(missing, "CLG_KEK_BASE64")
	}
	if c.JWT.Secret == "" {
		missing = append(missing, "CLG_JWT_SECRET")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}
	return nil
}

// ValidateForGateway runs additional validation on top of validate() for the
// gateway process (which needs LMS settings, cloud creds, etc.). Worker has a
// slightly different requirement set — but for hackathon scope both validate
// identically.
func (c Config) ValidateForGateway() error {
	if err := c.validate(); err != nil {
		return err
	}
	if c.OpenStack.AuthURL == "" {
		// Soft requirement: gateway can boot in mock mode (e.g. demo without КИ).
		return nil
	}
	if c.OpenStack.Password == "" {
		return errors.New("OPENSTACK_AUTH_URL set but OPENSTACK_PASSWORD missing")
	}
	return nil
}

func getString(v *viper.Viper, key, def string) string {
	if val := v.GetString(key); val != "" {
		return val
	}
	return def
}

func getInt(v *viper.Viper, key string, def int) int {
	val := v.GetString(key)
	if val == "" {
		return def
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return n
}

func getFloat(v *viper.Viper, key string, def float64) float64 {
	val := v.GetString(key)
	if val == "" {
		return def
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return def
	}
	return f
}

func getBool(v *viper.Viper, key string, def bool) bool {
	val := strings.ToLower(v.GetString(key))
	switch val {
	case "":
		return def
	case "true", "1", "yes", "y":
		return true
	case "false", "0", "no", "n":
		return false
	default:
		return def
	}
}
