package flexera

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

// Client reads plugin configuration and resolves billing-center tags.
// Cost RPCs use internal/flexeraapi, not this client.
type Client struct {
	cfg        Config
	bcResolver *BillingCenterResolver
}

// NewClient builds a client and, when mappings are configured, a billing-center resolver.
func NewClient(_ context.Context, cfg Config) (*Client, error) {
	client := &Client{cfg: cfg}
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

// CostPoint is one legacy cost row enriched with a billing center id.
type CostPoint struct {
	Timestamp    string             `json:"timestamp"`
	Dimensions   map[string]string  `json:"dimensions"`
	Metrics      map[string]float64 `json:"metrics"`
	Currency     string             `json:"currency"`
	Vendor       string             `json:"vendor,omitempty"`
	Service      string             `json:"service,omitempty"`
	Region       string             `json:"region,omitempty"`
	Account      string             `json:"vendor_account,omitempty"`
	ResourceType string             `json:"resource_type,omitempty"`
}

// EnrichCostPointWithBillingCenter adds a billing center id from tag dimensions.
func (c *Client) EnrichCostPointWithBillingCenter(point *CostPoint) {
	if c == nil || c.bcResolver == nil || point == nil {
		return
	}
	tags := make(map[string]string)
	for key, value := range point.Dimensions {
		if strings.HasPrefix(key, "tag:") {
			tags[strings.TrimPrefix(key, "tag:")] = value
		}
	}
	if bcID, err := c.bcResolver.ResolveBillingCenter(tags); err == nil && bcID != "" {
		if point.Dimensions == nil {
			point.Dimensions = map[string]string{}
		}
		point.Dimensions["billing_center"] = bcID
	}
}

// CostTagDimensions returns the costs/select dimension for each configured tag key.
// Bill Analysis names those dimensions tag_<key>.
func (c *Client) CostTagDimensions() []string {
	if c == nil || c.cfg.BillingCenterMappings == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var dims []string
	add := func(key string) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}
		if colon := strings.IndexByte(key, ':'); colon >= 0 {
			key = strings.TrimSpace(key[:colon])
		}
		if key == "" {
			return
		}
		dim := "tag_" + key
		if _, ok := seen[dim]; ok {
			return
		}
		seen[dim] = struct{}{}
		dims = append(dims, dim)
	}
	mapping := c.cfg.BillingCenterMappings
	for tagKey := range mapping.TagMappings {
		add(tagKey)
	}
	for _, hierarchy := range mapping.HierarchicalMappings {
		add(hierarchy.TagKey)
	}
	sort.Strings(dims)
	return dims
}

// GetBillingCenterForTags resolves a billing center ID from a set of tags.
func (c *Client) GetBillingCenterForTags(tags map[string]string) (string, error) {
	if c == nil || c.bcResolver == nil {
		return "", errors.New("billing center mappings not configured")
	}
	return c.bcResolver.ResolveBillingCenter(tags)
}

// GenerateFlexeraRBDConfig creates a rule-based dimension configuration for billing center allocation.
func (c *Client) GenerateFlexeraRBDConfig() (*RBDConfig, error) {
	if c == nil || c.bcResolver == nil {
		return nil, errors.New("billing center mappings not configured")
	}
	return &RBDConfig{
		RuleBasedDimensions: []RuleBasedDimension{{
			ID:   "rbd_bc",
			Name: "Billing Center Allocation",
			DatedRules: []DatedRules{{
				EffectiveAt: time.Now().UTC().Format("2006-01"),
				Rules:       c.bcResolver.BuildFlexeraRBDRules(),
			}},
		}},
	}, nil
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
