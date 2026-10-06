package flexera

import (
	"testing"
)

func TestBillingCenterResolver_ResolveBillingCenter(t *testing.T) {
	// Create test configuration
	config := BillingCenterMapping{
		TagMappings: map[string]string{
			"environment:production":  "bc-prod-123",
			"environment:staging":     "bc-staging-456",
			"environment:development": "bc-dev-789",
			"team:platform":           "bc-platform-abc",
			"cost-center:1001":        "bc-finance-ghi",
		},
		HierarchicalMappings: []TagHierarchy{
			{
				Priority: 1,
				TagKey:   "department",
				Mappings: map[string]string{
					"engineering": "bc-engineering-main",
					"finance":     "bc-finance-main",
				},
			},
			{
				Priority: 2,
				TagKey:   "project",
				Mappings: map[string]string{
					"alpha": "bc-project-alpha",
					"beta":  "bc-project-beta",
				},
			},
		},
		DefaultBillingCenter: "bc-default",
	}

	resolver := NewBillingCenterResolver(config, nil)

	tests := []struct {
		name     string
		tags     map[string]string
		expected string
		wantErr  bool
	}{
		{
			name: "Direct tag mapping - environment:production",
			tags: map[string]string{
				"environment": "production",
				"team":        "frontend",
			},
			expected: "bc-prod-123",
			wantErr:  false,
		},
		{
			name: "Direct tag mapping - team:platform",
			tags: map[string]string{
				"team": "platform",
			},
			expected: "bc-platform-abc",
			wantErr:  false,
		},
		{
			name: "Hierarchical mapping - department",
			tags: map[string]string{
				"department": "engineering",
				"owner":      "john.doe",
			},
			expected: "bc-engineering-main",
			wantErr:  false,
		},
		{
			name: "Hierarchical mapping - project (lower priority)",
			tags: map[string]string{
				"project": "alpha",
				"owner":   "jane.doe",
			},
			expected: "bc-project-alpha",
			wantErr:  false,
		},
		{
			name: "Priority test - department overrides project",
			tags: map[string]string{
				"department": "finance",
				"project":    "alpha",
			},
			expected: "bc-finance-main",
			wantErr:  false,
		},
		{
			name: "Direct mapping overrides hierarchical",
			tags: map[string]string{
				"environment": "staging",
				"department":  "engineering",
			},
			expected: "bc-staging-456",
			wantErr:  false,
		},
		{
			name: "Default billing center",
			tags: map[string]string{
				"random": "value",
				"other":  "tag",
			},
			expected: "bc-default",
			wantErr:  false,
		},
		{
			name:     "Empty tags - use default",
			tags:     map[string]string{},
			expected: "bc-default",
			wantErr:  false,
		},
		{
			name: "Cost center mapping",
			tags: map[string]string{
				"cost-center": "1001",
				"team":        "random",
			},
			expected: "bc-finance-ghi",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolver.ResolveBillingCenter(tt.tags)

			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveBillingCenter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if result != tt.expected {
				t.Errorf("ResolveBillingCenter() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBillingCenterResolver_BuildFlexeraRBDRules(t *testing.T) {
	config := BillingCenterMapping{
		TagMappings: map[string]string{
			"environment:production": "bc-prod-123",
			"team:platform":          "bc-platform-abc",
		},
		HierarchicalMappings: []TagHierarchy{
			{
				Priority: 1,
				TagKey:   "department",
				Mappings: map[string]string{
					"engineering": "bc-engineering-main",
				},
			},
		},
		DefaultBillingCenter: "bc-default",
	}

	resolver := NewBillingCenterResolver(config, nil)
	rules := resolver.BuildFlexeraRBDRules()

	// Check that we have the expected number of rules
	// 2 from tagMappings + 1 from hierarchical + 1 default = 4
	expectedRuleCount := 4
	if len(rules) != expectedRuleCount {
		t.Errorf("Expected %d rules, got %d", expectedRuleCount, len(rules))
	}

	// Verify specific rules are present
	hasEnvProdRule := false
	hasDefaultRule := false

	for _, rule := range rules {
		// Check for environment:production rule
		if rule.Condition.Dimension == "tag:environment" &&
			rule.Condition.Value == "production" &&
			rule.Value.Text == "bc-prod-123" {
			hasEnvProdRule = true
		}

		// Check for default rule (empty condition)
		if rule.Condition.Type == "" &&
			rule.Condition.Dimension == "" &&
			rule.Value.Text == "bc-default" {
			hasDefaultRule = true
		}
	}

	if !hasEnvProdRule {
		t.Error("Expected to find environment:production rule")
	}

	if !hasDefaultRule {
		t.Error("Expected to find default rule")
	}
}

func TestBillingCenterResolver_NoDefaultBillingCenter(t *testing.T) {
	// Test when no default billing center is configured
	config := BillingCenterMapping{
		TagMappings: map[string]string{
			"environment:production": "bc-prod-123",
		},
		DefaultBillingCenter: "", // No default
	}

	resolver := NewBillingCenterResolver(config, nil)

	// Tags that don't match should return an error
	_, err := resolver.ResolveBillingCenter(map[string]string{
		"unknown": "tag",
	})

	if err == nil {
		t.Error("Expected error when no billing center mapping found and no default configured")
	}
}

func TestClient_EnrichCostPointWithBillingCenter(t *testing.T) {
	// Create a client with billing center mappings
	cfg := Config{
		BaseURL:  "https://api.optima.flexeraeng.com/bill-analysis",
		OrgID:    "test-org",
		APIToken: "test-token",
		BillingCenterMappings: &BillingCenterMapping{
			TagMappings: map[string]string{
				"environment:production": "bc-prod-123",
				"team:platform":          "bc-platform-abc",
			},
			DefaultBillingCenter: "bc-default",
		},
	}

	client, err := NewClient(nil, cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test cost point with tags that should map to billing center
	point := &CostPoint{
		Timestamp: "2025-01-01",
		Dimensions: map[string]string{
			"vendor":          "AWS",
			"service":         "EC2",
			"tag:environment": "production",
			"tag:team":        "frontend",
		},
		Metrics: map[string]float64{
			"cost": 100.0,
		},
	}

	client.EnrichCostPointWithBillingCenter(point)

	// Check that billing center was added
	if bc, exists := point.Dimensions["billing_center"]; !exists {
		t.Error("Expected billing_center to be added to dimensions")
	} else if bc != "bc-prod-123" {
		t.Errorf("Expected billing_center to be 'bc-prod-123', got '%s'", bc)
	}
}

func TestClient_GetBillingCenterForTags(t *testing.T) {
	cfg := Config{
		BaseURL:  "https://api.optima.flexeraeng.com/bill-analysis",
		OrgID:    "test-org",
		APIToken: "test-token",
		BillingCenterMappings: &BillingCenterMapping{
			TagMappings: map[string]string{
				"environment:production": "bc-prod-123",
			},
			DefaultBillingCenter: "bc-default",
		},
	}

	client, err := NewClient(nil, cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test with matching tags
	bcID, err := client.GetBillingCenterForTags(map[string]string{
		"environment": "production",
	})

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if bcID != "bc-prod-123" {
		t.Errorf("Expected 'bc-prod-123', got '%s'", bcID)
	}

	// Test with non-matching tags (should use default)
	bcID, err = client.GetBillingCenterForTags(map[string]string{
		"random": "tag",
	})

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if bcID != "bc-default" {
		t.Errorf("Expected 'bc-default', got '%s'", bcID)
	}
}
