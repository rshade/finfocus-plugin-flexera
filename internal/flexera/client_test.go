package flexera

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseResourceID(t *testing.T) {
	tests := []struct {
		resourceID string
		expected   map[string]string
	}{
		{
			"vendor_account/123456789",
			map[string]string{"vendor_account": "123456789"},
		},
		{
			"service/aws/ec2",
			map[string]string{"vendor": "aws", "service": "ec2"},
		},
		{
			"region/azure/eastus",
			map[string]string{"vendor": "azure", "region": "eastus"},
		},
		{
			"resource_group/my-resource-group",
			map[string]string{"resource_group": "my-resource-group"},
		},
		{
			"billing_center/bc-123",
			map[string]string{"rbd_bc": "bc-123"},
		},
		{
			"tag/environment/production",
			map[string]string{"tag:environment": "production"},
		},
		{
			"invalid",
			map[string]string{},
		},
	}

	for _, test := range tests {
		result := parseResourceID(test.resourceID)
		if len(result) != len(test.expected) {
			t.Errorf(
				"For resourceID '%s', expected %d filters, got %d",
				test.resourceID,
				len(test.expected),
				len(result),
			)
			continue
		}

		for key, expectedValue := range test.expected {
			if actualValue, exists := result[key]; !exists || actualValue != expectedValue {
				t.Errorf(
					"For resourceID '%s', expected filter '%s': '%s', got '%s'",
					test.resourceID,
					key,
					expectedValue,
					actualValue,
				)
			}
		}
	}
}

func TestFormatDateForFlexera(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"2025-01-15", "2025-01-15"},
		{"2025-01-15T10:30:00Z", "2025-01-15"},
		{"2025-01-15T10:30:00.000Z", "2025-01-15"},
		{"", time.Now().UTC().Format("2006-01-02")}, // Should return today's date
	}

	for _, test := range tests {
		result := formatDateForFlexera(test.input)
		if test.input == "" {
			// For empty input, just check the format
			if len(result) != 10 || result[4] != '-' || result[7] != '-' {
				t.Errorf("For empty input, expected date format YYYY-MM-DD, got '%s'", result)
			}
		} else {
			if result != test.expected {
				t.Errorf("For input '%s', expected '%s', got '%s'", test.input, test.expected, result)
			}
		}
	}
}

func TestBuildCostQuery(t *testing.T) {
	cfg := Config{
		BaseURL: "https://api.optima.flexeraeng.com/bill-analysis",
		OrgID:   "test-org",
	}

	client := &Client{cfg: cfg}

	query := client.BuildCostQuery("vendor_account/123456", "2025-01-01T00:00:00Z", "2025-01-31T23:59:59Z")

	// Verify basic query structure
	if query.StartAt != "2025-01-01" {
		t.Errorf("Expected startAt '2025-01-01', got '%s'", query.StartAt)
	}
	if query.EndAt != "2025-01-31" {
		t.Errorf("Expected endAt '2025-01-31', got '%s'", query.EndAt)
	}
	if query.Granularity != "day" {
		t.Errorf("Expected granularity 'day', got '%s'", query.Granularity)
	}
	if len(query.Metrics) != 1 || query.Metrics[0] != "cost_amortized_unblended_adj" {
		t.Errorf("Expected metrics [cost_amortized_unblended_adj], got %v", query.Metrics)
	}

	// Verify filter was applied correctly
	if vendorAccount, exists := query.Filter["vendor_account"]; !exists || vendorAccount != "123456" {
		t.Errorf("Expected filter vendor_account: '123456', got %v", query.Filter)
	}
}

func TestCostsAPICall(t *testing.T) {
	// Mock server to simulate Flexera API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type 'application/json', got '%s'", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Api-Version") != "1.0" {
			t.Errorf("Expected Api-Version '1.0', got '%s'", r.Header.Get("Api-Version"))
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Expected Authorization 'Bearer test-token', got '%s'", r.Header.Get("Authorization"))
		}

		// Return mock response
		response := CostResponse{
			Results: []CostPoint{
				{
					Timestamp: "2025-01-01",
					Dimensions: map[string]string{
						"vendor":  "AWS",
						"service": "EC2",
					},
					Metrics: map[string]float64{
						"cost_amortized_unblended_adj": 150.75,
					},
					Currency: "USD",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create client with mock server URL
	cfg := Config{
		BaseURL:  server.URL,
		OrgID:    "test-org",
		APIToken: "test-token",
		Timeout:  5 * time.Second,
	}

	client, err := NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Override the costs URL to point to our mock server
	client.cfg.BaseURL = server.URL

	query := CostQuery{
		StartAt:     "2025-01-01",
		EndAt:       "2025-01-31",
		Granularity: "day",
		Metrics:     []string{"cost_amortized_unblended_adj"},
		Dimensions:  []string{"vendor", "service"},
		Filter:      map[string]string{"vendor": "AWS"},
	}

	response, err := client.Costs(context.Background(), query)
	if err != nil {
		t.Fatalf("Costs API call failed: %v", err)
	}

	// Verify response
	if len(response.Results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(response.Results))
	}

	result := response.Results[0]
	if result.Timestamp != "2025-01-01" {
		t.Errorf("Expected timestamp '2025-01-01', got '%s'", result.Timestamp)
	}
	if cost := result.Metrics["cost_amortized_unblended_adj"]; cost != 150.75 {
		t.Errorf("Expected cost 150.75, got %f", cost)
	}
}
