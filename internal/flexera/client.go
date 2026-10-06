package flexera

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	cfg        Config
	http       *http.Client
	bcResolver *BillingCenterResolver
}

const (
	httpErrorStatus  = 300
	defaultCostLimit = 1000
	minIDParts       = 2
	serviceIDParts   = 3
)

func NewClient(_ context.Context, cfg Config) (*Client, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.TLSSkipVerify {
		tlsConfig.InsecureSkipVerify = true
	}
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	client := &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
		},
	}

	// Initialize billing center resolver if mappings are configured
	if cfg.BillingCenterMappings != nil {
		client.bcResolver = NewBillingCenterResolver(*cfg.BillingCenterMappings, client)
	}

	return client, nil
}

// Region returns the configured Flexera deployment region (nam, eu, or apac).
func (c *Client) Region() string {
	if c == nil {
		return ""
	}
	return c.cfg.Region
}

// CostQuery represents a request to the Flexera Optima costs API.
type CostQuery struct {
	StartAt     string            `json:"start_at"`    // "2025-01-01"
	EndAt       string            `json:"end_at"`      // "2025-01-31"
	Granularity string            `json:"granularity"` // "day", "month"
	Metrics     []string          `json:"metrics"`     // ["cost_amortized_unblended_adj"]
	Dimensions  []string          `json:"dimensions"`  // ["vendor", "service", "region"]
	Filter      map[string]string `json:"filter"`      // filtering criteria
	Limit       int               `json:"limit,omitempty"`
}

// CostPoint represents a single cost data point from Flexera.
type CostPoint struct {
	Timestamp  string             `json:"timestamp"`
	Dimensions map[string]string  `json:"dimensions"`
	Metrics    map[string]float64 `json:"metrics"`
	Currency   string             `json:"currency"`
	// Flexera-specific fields
	Vendor       string `json:"vendor,omitempty"`
	Service      string `json:"service,omitempty"`
	Region       string `json:"region,omitempty"`
	Account      string `json:"vendor_account,omitempty"`
	ResourceType string `json:"resource_type,omitempty"`
}

// CostResponse represents the response from Flexera costs API.
type CostResponse struct {
	Results []CostPoint `json:"results"`
	Meta    struct {
		Pagination struct {
			TotalCount int `json:"total_count"`
			Limit      int `json:"limit"`
			Offset     int `json:"offset"`
		} `json:"pagination"`
	} `json:"meta"`
}

// Costs queries the Flexera Optima costs API.
func (c *Client) Costs(ctx context.Context, q CostQuery) (CostResponse, error) {
	reqURL := c.cfg.GetCostsURL()

	// Build request body
	reqData, err := json.Marshal(q)
	if err != nil {
		return CostResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(string(reqData)))
	if err != nil {
		return CostResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers for Flexera API
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIToken)
	req.Header.Set("Api-Version", "1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return CostResponse{}, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= httpErrorStatus {
		return CostResponse{}, fmt.Errorf("flexera api error: %d %s", resp.StatusCode, resp.Status)
	}

	var out CostResponse
	if decodeErr := json.NewDecoder(resp.Body).Decode(&out); decodeErr != nil {
		return out, fmt.Errorf("failed to decode response: %w", decodeErr)
	}

	// Enrich each cost point with billing center information based on tags
	for i := range out.Results {
		c.EnrichCostPointWithBillingCenter(&out.Results[i])
	}

	return out, nil
}

// BuildCostQuery creates a CostQuery from resource ID and time range.
func (c *Client) BuildCostQuery(resourceID, startTime, endTime string) CostQuery {
	query := CostQuery{
		StartAt:     formatDateForFlexera(startTime),
		EndAt:       formatDateForFlexera(endTime),
		Granularity: "day",
		Metrics:     []string{"cost_amortized_unblended_adj"},
		Dimensions:  []string{"vendor", "service", "region", "vendor_account"},
		Filter:      make(map[string]string),
		Limit:       defaultCostLimit,
	}

	// Parse resourceID and set appropriate filters
	if resourceID != "" {
		filters := parseResourceID(resourceID)
		for k, v := range filters {
			query.Filter[k] = v
		}
	}

	return query
}

// parseResourceID converts Pulumi-style resource IDs to Flexera filters.
func parseResourceID(resourceID string) map[string]string {
	filters := make(map[string]string)

	parts := strings.Split(resourceID, "/")
	if len(parts) < minIDParts {
		return filters
	}

	switch parts[0] {
	case "vendor_account":
		if len(parts) >= minIDParts {
			filters["vendor_account"] = parts[1]
		}
	case "service":
		if len(parts) >= serviceIDParts {
			filters["vendor"] = parts[1]
			filters["service"] = parts[2]
		}
	case "region":
		if len(parts) >= serviceIDParts {
			filters["vendor"] = parts[1]
			filters["region"] = parts[2]
		}
	case "resource_group":
		if len(parts) >= minIDParts {
			filters["resource_group"] = parts[1]
		}
	case "billing_center":
		if len(parts) >= minIDParts {
			filters["rbd_bc"] = parts[1] // Rule-based dimension for billing center
		}
	case "tag":
		if len(parts) >= serviceIDParts {
			filters[fmt.Sprintf("tag:%s", parts[1])] = parts[2]
		}
	}

	return filters
}

// EnrichCostPointWithBillingCenter adds billing center information to a cost point based on tags.
func (c *Client) EnrichCostPointWithBillingCenter(point *CostPoint) {
	if c.bcResolver == nil {
		return // No billing center mappings configured
	}

	// Extract tags from dimensions
	tags := make(map[string]string)
	for key, value := range point.Dimensions {
		if strings.HasPrefix(key, "tag:") {
			tagKey := strings.TrimPrefix(key, "tag:")
			tags[tagKey] = value
		}
	}

	// Resolve billing center from tags
	if bcID, err := c.bcResolver.ResolveBillingCenter(tags); err == nil && bcID != "" {
		point.Dimensions["billing_center"] = bcID
	}
}

// GetBillingCenterForTags resolves a billing center ID from a set of tags.
func (c *Client) GetBillingCenterForTags(tags map[string]string) (string, error) {
	if c.bcResolver == nil {
		return "", errors.New("billing center mappings not configured")
	}
	return c.bcResolver.ResolveBillingCenter(tags)
}

// formatDateForFlexera converts time strings to Flexera API format.
func formatDateForFlexera(timeStr string) string {
	if timeStr == "" {
		return time.Now().UTC().Format("2006-01-02")
	}

	// Try to parse common formats and convert to YYYY-MM-DD
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t.Format("2006-01-02")
		}
	}

	// If parsing fails, return as-is
	return timeStr
}

// GetBudgets retrieves budget information from Flexera.
func (c *Client) GetBudgets(ctx context.Context) (interface{}, error) {
	budgetURL := fmt.Sprintf("%s/orgs/%s/budgets", c.cfg.BaseURL, c.cfg.OrgID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, budgetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create budget request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.cfg.APIToken)
	req.Header.Set("Api-Version", "1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get budgets: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= httpErrorStatus {
		return nil, fmt.Errorf("flexera budget api error: %d %s", resp.StatusCode, resp.Status)
	}

	var result interface{}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&result); decodeErr != nil {
		return nil, fmt.Errorf("failed to decode budget response: %w", decodeErr)
	}

	return result, nil
}

// GetCloudAccounts retrieves cloud account information.
func (c *Client) GetCloudAccounts(ctx context.Context, vendor string) (interface{}, error) {
	accountsURL := fmt.Sprintf("%s/orgs/%s/cloud_accounts", c.cfg.BaseURL, c.cfg.OrgID)

	if vendor != "" {
		params := url.Values{}
		params.Add("vendor", vendor)
		accountsURL += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, accountsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create accounts request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.cfg.APIToken)
	req.Header.Set("Api-Version", "1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get cloud accounts: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= httpErrorStatus {
		return nil, fmt.Errorf("flexera accounts api error: %d %s", resp.StatusCode, resp.Status)
	}

	var result interface{}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&result); decodeErr != nil {
		return nil, fmt.Errorf("failed to decode accounts response: %w", decodeErr)
	}

	return result, nil
}

// GenerateFlexeraRBDConfig creates a complete Flexera rule-based dimension configuration
// for billing center allocation based on the configured tag mappings.
func (c *Client) GenerateFlexeraRBDConfig() (*RBDConfig, error) {
	if c.bcResolver == nil {
		return nil, errors.New("billing center mappings not configured")
	}

	rules := c.bcResolver.BuildFlexeraRBDRules()

	config := &RBDConfig{
		RuleBasedDimensions: []RuleBasedDimension{
			{
				ID:   "rbd_bc",
				Name: "Billing Center Allocation",
				DatedRules: []DatedRules{
					{
						EffectiveAt: time.Now().UTC().Format("2006-01"),
						Rules:       rules,
					},
				},
			},
		},
	}

	return config, nil
}

// RBDConfig represents the complete rule-based dimension configuration for Flexera.
type RBDConfig struct {
	RuleBasedDimensions []RuleBasedDimension `json:"rule_based_dimensions"`
}

// RuleBasedDimension represents a single rule-based dimension in Flexera.
type RuleBasedDimension struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	DatedRules []DatedRules `json:"dated_rules"`
}

// DatedRules represents rules that are effective from a specific date.
type DatedRules struct {
	EffectiveAt string    `json:"effective_at"`
	Rules       []RBDRule `json:"rules"`
}
