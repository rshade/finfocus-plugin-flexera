package flexera

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRegionAndBillingHelpers(t *testing.T) {
	if (*Client)(nil).Region() != "" {
		t.Fatal("nil client region")
	}
	client := &Client{cfg: Config{Region: "eu"}}
	if client.Region() != "eu" {
		t.Fatalf("region = %q", client.Region())
	}
	client.EnrichCostPointWithBillingCenter(nil)
	if _, err := client.GetBillingCenterForTags(map[string]string{"environment": "prod"}); err == nil {
		t.Fatal("expected missing resolver error")
	}
	if _, err := client.GenerateFlexeraRBDConfig(); err == nil {
		t.Fatal("expected missing resolver error")
	}

	mapped, err := NewClient(context.Background(), Config{
		OrgID: "1",
		BillingCenterMappings: &BillingCenterMapping{
			TagMappings:          map[string]string{"environment:production": "bc-prod-123"},
			DefaultBillingCenter: "bc-default",
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if dims := mapped.CostTagDimensions(); len(dims) != 1 || dims[0] != "tag_environment" {
		t.Fatalf("tag dimensions = %#v", dims)
	}
	if id, idErr := mapped.GetBillingCenterForTags(nil); idErr != nil || id != "bc-default" {
		t.Fatalf("default billing center = %q err=%v", id, idErr)
	}
	point := &CostPoint{}
	mapped.EnrichCostPointWithBillingCenter(point)
	if point.Dimensions["billing_center"] != "bc-default" {
		t.Fatalf("dimensions = %#v", point.Dimensions)
	}
	config, err := mapped.GenerateFlexeraRBDConfig()
	if err != nil || len(config.RuleBasedDimensions) != 1 {
		t.Fatalf("rbd = %#v err=%v", config, err)
	}
}

func TestLookupBillingCenterByName(t *testing.T) {
	resolver := NewBillingCenterResolver(BillingCenterMapping{
		EnableAutoDiscovery: true,
		HierarchicalMappings: []TagHierarchy{{
			TagKey:   "department",
			Mappings: map[string]string{"finance": "bc-finance"},
		}},
		DefaultBillingCenter: "bc-default",
	}, nil)
	id, err := resolver.ResolveBillingCenter(map[string]string{"department": "unknown-dept"})
	if err != nil || id != "bc-default" {
		t.Fatalf("id=%q err=%v", id, err)
	}

	resolver.bcCache["platform"] = "bc-cached"
	cached, err := resolver.lookupBillingCenterByName("platform")
	if err != nil || cached != "bc-cached" {
		t.Fatalf("cached=%q err=%v", cached, err)
	}
	if _, err := resolver.lookupBillingCenterByName("missing"); err == nil {
		t.Fatal("expected lookup error")
	}
}

func TestExampleBillingCenterConfig(t *testing.T) {
	cfg := ExampleBillingCenterConfig()
	if cfg.TagMappings["environment:production"] != "bc-prod-123" || !cfg.EnableAutoDiscovery {
		t.Fatalf("example = %#v", cfg)
	}
}

func TestHasOAuth(t *testing.T) {
	if (Config{RefreshToken: "rt"}).HasOAuth() != true {
		t.Fatal("refresh token")
	}
	if (Config{ClientID: "id", ClientSecret: "secret"}).HasOAuth() != true {
		t.Fatal("client credentials")
	}
	if (Config{ClientID: "id"}).HasOAuth() {
		t.Fatal("client id alone")
	}
	if !(Config{APIToken: "legacy"}).HasCostCredentials() {
		t.Fatal("legacy token")
	}
}

func TestLoadConfigFileAndOverrides(t *testing.T) {
	t.Setenv("FLEXERA_ORG_ID", "42")
	t.Setenv("FLEXERA_REFRESH_TOKEN", "env-refresh")
	t.Setenv("FLEXERA_CLIENT_ID", "env-id")
	t.Setenv("FLEXERA_CLIENT_SECRET", "env-secret")
	t.Setenv("FLEXERA_API_TOKEN", "env-token")
	t.Setenv("FLEXERA_COST_METRIC", "cost_list")
	t.Setenv("FLEXERA_BILLING_CENTER_IDS", " bc-1, ,bc-2 ")
	t.Setenv("FLEXERA_REGION", "apac")
	t.Setenv("FLEXERA_TIMEOUT", "not-a-duration")
	t.Setenv("FLEXERA_TLS_SKIP_VERIFY", "true")

	missing, err := LoadConfigFromEnvOrFile(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil || missing.OrgID != "42" || len(missing.BillingCenterIDs) != 2 {
		t.Fatalf("missing file cfg=%#v err=%v", missing, err)
	}

	if _, err := LoadConfigFromEnvOrFile(t.TempDir()); err == nil {
		t.Fatal("expected directory read error")
	}

	path := filepath.Join(t.TempDir(), "flexera.yaml")
	body := []byte(
		"region: eu\norgId: \"7\"\nrefreshToken: file-token\ncostMetric: \"\"\nbaseUrl: https://example.invalid/bill\n",
	)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := LoadConfigFromEnvOrFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.RefreshToken != "env-refresh" || cfg.ClientSecret != "env-secret" || cfg.APIToken != "env-token" {
		t.Fatalf("env did not override file: %#v", cfg)
	}
	if cfg.CostMetric != "cost_list" || cfg.BaseURL != "https://example.invalid/bill" {
		t.Fatalf("metric or url = %#v", cfg)
	}
	if !cfg.TLSSkipVerify || cfg.Timeout != defaultTimeout {
		t.Fatalf("tls=%v timeout=%s", cfg.TLSSkipVerify, cfg.Timeout)
	}
}

func TestSplitCSV(t *testing.T) {
	if splitCSV("   ") != nil {
		t.Fatal("blank csv")
	}
	got := splitCSV(" a, ,b ")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("split = %#v", got)
	}
}
