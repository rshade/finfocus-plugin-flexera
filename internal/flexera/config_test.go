package flexera

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigFromEnv(t *testing.T) {
	// Set test environment variables
	os.Setenv("FLEXERA_REGION", "eu")
	os.Setenv("FLEXERA_ORG_ID", "test-org-123")
	os.Setenv("FLEXERA_API_TOKEN", "test-token-xyz")
	os.Setenv("FLEXERA_DEFAULT_WINDOW", "7d")
	os.Setenv("FLEXERA_TIMEOUT", "45s")

	defer func() {
		os.Unsetenv("FLEXERA_REGION")
		os.Unsetenv("FLEXERA_ORG_ID")
		os.Unsetenv("FLEXERA_API_TOKEN")
		os.Unsetenv("FLEXERA_DEFAULT_WINDOW")
		os.Unsetenv("FLEXERA_TIMEOUT")
	}()

	cfg, err := LoadConfigFromEnvOrFile("")
	if err != nil {
		t.Fatalf("LoadConfigFromEnvOrFile failed: %v", err)
	}

	// Verify configuration values
	if cfg.Region != "eu" {
		t.Errorf("Expected region 'eu', got '%s'", cfg.Region)
	}
	if cfg.OrgID != "test-org-123" {
		t.Errorf("Expected orgId 'test-org-123', got '%s'", cfg.OrgID)
	}
	if cfg.APIToken != "test-token-xyz" {
		t.Errorf("Expected apiToken 'test-token-xyz', got '%s'", cfg.APIToken)
	}
	if cfg.DefaultWindow != "7d" {
		t.Errorf("Expected defaultWindow '7d', got '%s'", cfg.DefaultWindow)
	}
	if cfg.Timeout != 45*time.Second {
		t.Errorf("Expected timeout 45s, got %v", cfg.Timeout)
	}

	// Verify auto-configured URL for EU region
	expectedURL := "https://api.optima-eu.flexeraeng.com/bill-analysis"
	if cfg.BaseURL != expectedURL {
		t.Errorf("Expected baseURL '%s', got '%s'", expectedURL, cfg.BaseURL)
	}
}

func TestGetBaseURLForRegion(t *testing.T) {
	tests := []struct {
		region   string
		expected string
	}{
		{"nam", "https://api.optima.flexeraeng.com/bill-analysis"},
		{"north-america", "https://api.optima.flexeraeng.com/bill-analysis"},
		{"eu", "https://api.optima-eu.flexeraeng.com/bill-analysis"},
		{"europe", "https://api.optima-eu.flexeraeng.com/bill-analysis"},
		{"apac", "https://api.optima-apac.flexeraeng.com/bill-analysis"},
		{"asia-pacific", "https://api.optima-apac.flexeraeng.com/bill-analysis"},
		{"unknown", "https://api.optima.flexeraeng.com/bill-analysis"}, // Default to NAM
	}

	for _, test := range tests {
		result := getBaseURLForRegion(test.region)
		if result != test.expected {
			t.Errorf("For region '%s', expected '%s', got '%s'", test.region, test.expected, result)
		}
	}
}

func TestGetCostsURL(t *testing.T) {
	cfg := Config{
		BaseURL: "https://api.optima.flexeraeng.com/bill-analysis",
		OrgID:   "test-org-456",
	}

	expectedURL := "https://api.optima.flexeraeng.com/bill-analysis/orgs/test-org-456/costs"
	result := cfg.GetCostsURL()

	if result != expectedURL {
		t.Errorf("Expected costs URL '%s', got '%s'", expectedURL, result)
	}
}

func TestLoadConfigValidation(t *testing.T) {
	// Test missing required fields
	os.Unsetenv("FLEXERA_ORG_ID")
	os.Unsetenv("FLEXERA_API_TOKEN")

	_, err := LoadConfigFromEnvOrFile("")
	if err == nil {
		t.Error("Expected error for missing required fields, got nil")
	}

	// Test missing only OrgID
	os.Setenv("FLEXERA_API_TOKEN", "test-token")
	defer os.Unsetenv("FLEXERA_API_TOKEN")

	_, err = LoadConfigFromEnvOrFile("")
	if err == nil || err.Error() != "FLEXERA_ORG_ID is required" {
		t.Errorf("Expected 'FLEXERA_ORG_ID is required' error, got: %v", err)
	}

	// Test missing only API token
	os.Setenv("FLEXERA_ORG_ID", "test-org")
	os.Unsetenv("FLEXERA_API_TOKEN")
	defer os.Unsetenv("FLEXERA_ORG_ID")

	_, err = LoadConfigFromEnvOrFile("")
	if err == nil || !strings.Contains(err.Error(), "FLEXERA_REFRESH_TOKEN") {
		t.Errorf("Expected a credential error, got: %v", err)
	}
}
