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
	Region        string        `yaml:"region"`        // nam, eu, apac
	OrgID         string        `yaml:"orgId"`
	APIToken      string        `yaml:"apiToken"`      // JWT token from Cloud Management API
	DefaultWindow string        `yaml:"defaultWindow"` // e.g. "30d"
	Timeout       time.Duration `yaml:"timeout"`
	TLSSkipVerify bool          `yaml:"tlsSkipVerify"`
	
	// BillingCenterMappings configures how to map resource tags to billing centers
	BillingCenterMappings *BillingCenterMapping `yaml:"billingCenterMappings,omitempty"`
}

func LoadConfigFromEnvOrFile(path string) (Config, error) {
	cfg := Config{
		BaseURL:       os.Getenv("FLEXERA_BASE_URL"),
		Region:        getenvDefault("FLEXERA_REGION", "nam"),
		OrgID:         os.Getenv("FLEXERA_ORG_ID"),
		APIToken:      os.Getenv("FLEXERA_API_TOKEN"),
		DefaultWindow: getenvDefault("FLEXERA_DEFAULT_WINDOW", "30d"),
		Timeout:       getenvDuration("FLEXERA_TIMEOUT", 30*time.Second),
		TLSSkipVerify: os.Getenv("FLEXERA_TLS_SKIP_VERIFY") == "true",
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
	
	// Validate required fields
	if cfg.OrgID == "" {
		return cfg, errors.New("FLEXERA_ORG_ID is required")
	}
	if cfg.APIToken == "" {
		return cfg, errors.New("FLEXERA_API_TOKEN is required")
	}
	
	return cfg, nil
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
