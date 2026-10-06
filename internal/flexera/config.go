package flexera

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	BaseURL       string        `yaml:"baseUrl"`
	Region        string        `yaml:"region"` // nam, eu, apac
	OrgID         string        `yaml:"orgId"`
	APIToken      string        `yaml:"apiToken"` // legacy bearer token; prefer refreshToken or client credentials
	RefreshToken  string        `yaml:"refreshToken"`
	ClientID      string        `yaml:"clientId"`
	ClientSecret  string        `yaml:"clientSecret"`
	DefaultWindow string        `yaml:"defaultWindow"` // e.g. "30d"
	Timeout       time.Duration `yaml:"timeout"`
	TLSSkipVerify bool          `yaml:"tlsSkipVerify"`
	CostMetric    string        `yaml:"costMetric"`

	// BillingCenterIDs are sent on every costs/select call. At least one is required for cost RPCs.
	BillingCenterIDs []string `yaml:"billingCenterIds"`

	// BillingCenterMappings configures how to map resource tags to billing centers
	BillingCenterMappings *BillingCenterMapping `yaml:"billingCenterMappings,omitempty"`
}

func LoadConfigFromEnvOrFile(path string) (Config, error) {
	cfg := Config{
		BaseURL:          os.Getenv("FLEXERA_BASE_URL"),
		Region:           getenvDefault("FLEXERA_REGION", "nam"),
		OrgID:            os.Getenv("FLEXERA_ORG_ID"),
		APIToken:         os.Getenv("FLEXERA_API_TOKEN"),
		RefreshToken:     os.Getenv("FLEXERA_REFRESH_TOKEN"),
		ClientID:         os.Getenv("FLEXERA_CLIENT_ID"),
		ClientSecret:     os.Getenv("FLEXERA_CLIENT_SECRET"),
		DefaultWindow:    getenvDefault("FLEXERA_DEFAULT_WINDOW", "30d"),
		Timeout:          getenvDuration("FLEXERA_TIMEOUT", 30*time.Second),
		TLSSkipVerify:    os.Getenv("FLEXERA_TLS_SKIP_VERIFY") == "true",
		CostMetric:       getenvDefault("FLEXERA_COST_METRIC", defaultCostMetric),
		BillingCenterIDs: splitCSV(os.Getenv("FLEXERA_BILLING_CENTER_IDS")),
	}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			// If file doesn't exist, just use environment/default values
			if !errors.Is(err, os.ErrNotExist) {
				return cfg, err
			}
		} else {
			_ = yaml.Unmarshal(b, &cfg)
		}
	}

	// Auto-configure BaseURL from region if not explicitly set
	if cfg.BaseURL == "" {
		cfg.BaseURL = getBaseURLForRegion(cfg.Region)
	}

	applyCredentialEnv(&cfg)
	if cfg.CostMetric == "" {
		cfg.CostMetric = defaultCostMetric
	}
	if len(cfg.BillingCenterIDs) == 0 {
		cfg.BillingCenterIDs = splitCSV(os.Getenv("FLEXERA_BILLING_CENTER_IDS"))
	}

	if cfg.OrgID == "" {
		return cfg, errors.New("FLEXERA_ORG_ID is required")
	}
	if !cfg.HasCostCredentials() {
		return cfg, errors.New(
			"FLEXERA_REFRESH_TOKEN, FLEXERA_CLIENT_ID and FLEXERA_CLIENT_SECRET, or FLEXERA_API_TOKEN is required",
		)
	}

	return cfg, nil
}

const defaultCostMetric = "cost_amortized_unblended_adj"

// HasCostCredentials reports whether config can authenticate a Flexera call.
func (c Config) HasCostCredentials() bool {
	if c.RefreshToken != "" || c.APIToken != "" {
		return true
	}
	return c.ClientID != "" && c.ClientSecret != ""
}

// HasOAuth reports whether config has a refresh token or a service account.
func (c Config) HasOAuth() bool {
	if c.RefreshToken != "" {
		return true
	}
	return c.ClientID != "" && c.ClientSecret != ""
}

func applyCredentialEnv(cfg *Config) {
	if v := os.Getenv("FLEXERA_REFRESH_TOKEN"); v != "" {
		cfg.RefreshToken = v
	}
	if v := os.Getenv("FLEXERA_CLIENT_ID"); v != "" {
		cfg.ClientID = v
	}
	if v := os.Getenv("FLEXERA_CLIENT_SECRET"); v != "" {
		cfg.ClientSecret = v
	}
	if v := os.Getenv("FLEXERA_API_TOKEN"); v != "" {
		cfg.APIToken = v
	}
	if v := os.Getenv("FLEXERA_COST_METRIC"); v != "" {
		cfg.CostMetric = v
	}
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func getenvDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvDuration(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// getBaseURLForRegion returns the appropriate Flexera Optima API endpoint for the region
func getBaseURLForRegion(region string) string {
	switch strings.ToLower(region) {
	case "nam", "north-america":
		return "https://api.optima.flexeraeng.com/bill-analysis"
	case "eu", "europe":
		return "https://api.optima-eu.flexeraeng.com/bill-analysis"
	case "apac", "asia-pacific":
		return "https://api.optima-apac.flexeraeng.com/bill-analysis"
	default:
		// Default to NAM
		return "https://api.optima.flexeraeng.com/bill-analysis"
	}
}

// GetCostsURL returns the full URL for the costs endpoint
func (c Config) GetCostsURL() string {
	return fmt.Sprintf("%s/orgs/%s/costs", c.BaseURL, c.OrgID)
}
