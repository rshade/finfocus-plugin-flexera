package flexera

import (
	"fmt"
	"strings"
)

// BillingCenterMapping defines how to map tags to billing centers
type BillingCenterMapping struct {
	// TagMappings maps tag key/value pairs to billing center IDs
	// Example: {"environment:production": "bc-prod-123", "team:finance": "bc-finance-456"}
	TagMappings map[string]string `yaml:"tagMappings" json:"tagMappings"`
	
	// HierarchicalMappings allows hierarchical tag lookups
	// First matching tag wins (ordered by priority)
	HierarchicalMappings []TagHierarchy `yaml:"hierarchicalMappings" json:"hierarchicalMappings"`
	
	// DefaultBillingCenter is used when no tags match
	DefaultBillingCenter string `yaml:"defaultBillingCenter" json:"defaultBillingCenter"`
	
	// EnableAutoDiscovery will query Flexera for billing center IDs by name
	EnableAutoDiscovery bool `yaml:"enableAutoDiscovery" json:"enableAutoDiscovery"`
}

// TagHierarchy defines a prioritized tag mapping rule
type TagHierarchy struct {
	Priority    int               `yaml:"priority" json:"priority"`
	TagKey      string            `yaml:"tagKey" json:"tagKey"`
	Mappings    map[string]string `yaml:"mappings" json:"mappings"`
	Description string            `yaml:"description" json:"description"`
}

// BillingCenterResolver handles the logic for resolving billing centers from tags
type BillingCenterResolver struct {
	config   BillingCenterMapping
	client   *Client
	bcCache  map[string]string // Cache of billing center name -> ID mappings
}

// NewBillingCenterResolver creates a new billing center resolver
func NewBillingCenterResolver(config BillingCenterMapping, client *Client) *BillingCenterResolver {
	return &BillingCenterResolver{
		config:  config,
		client:  client,
		bcCache: make(map[string]string),
	}
}

// ResolveBillingCenter determines the billing center ID from resource tags
func (r *BillingCenterResolver) ResolveBillingCenter(tags map[string]string) (string, error) {
	// First, check direct tag mappings
	for tagKey, tagValue := range tags {
		// Check combined key:value mapping
		combinedKey := fmt.Sprintf("%s:%s", tagKey, tagValue)
		if bcID, exists := r.config.TagMappings[combinedKey]; exists {
			return bcID, nil
		}
		
		// Check if just the tag key has a default mapping
		if bcID, exists := r.config.TagMappings[tagKey]; exists {
			return bcID, nil
		}
	}
	
	// Second, check hierarchical mappings (in priority order)
	for _, hierarchy := range r.config.HierarchicalMappings {
		if tagValue, exists := tags[hierarchy.TagKey]; exists {
			if bcID, mapped := hierarchy.Mappings[tagValue]; mapped {
				return bcID, nil
			}
			
			// If auto-discovery is enabled, try to find billing center by name
			if r.config.EnableAutoDiscovery {
				bcID, err := r.lookupBillingCenterByName(tagValue)
				if err == nil && bcID != "" {
					// Cache the result for future use
					r.bcCache[tagValue] = bcID
					return bcID, nil
				}
			}
		}
	}
	
	// Finally, return default billing center if configured
	if r.config.DefaultBillingCenter != "" {
		return r.config.DefaultBillingCenter, nil
	}
	
	return "", fmt.Errorf("no billing center mapping found for tags: %v", tags)
}

// lookupBillingCenterByName queries Flexera to find billing center ID by name
func (r *BillingCenterResolver) lookupBillingCenterByName(name string) (string, error) {
	// Check cache first
	if bcID, exists := r.bcCache[name]; exists {
		return bcID, nil
	}
	
	// In a real implementation, this would call the Flexera API
	// to search for billing centers by name
	// For now, we'll return an empty string
	return "", fmt.Errorf("billing center lookup not implemented for name: %s", name)
}

// BuildFlexeraRBDRules creates Flexera rule-based dimension rules for billing center allocation
func (r *BillingCenterResolver) BuildFlexeraRBDRules() []RBDRule {
	var rules []RBDRule
	
	// Create rules from direct tag mappings
	for tagMapping, bcID := range r.config.TagMappings {
		parts := strings.Split(tagMapping, ":")
		if len(parts) == 2 {
			rule := RBDRule{
				Condition: RBDCondition{
					Type:      "dimension_equals",
					Dimension: fmt.Sprintf("tag:%s", parts[0]),
					Value:     parts[1],
				},
				Value: RBDValue{
					Text: bcID,
				},
			}
			rules = append(rules, rule)
		}
	}
	
	// Create rules from hierarchical mappings
	for _, hierarchy := range r.config.HierarchicalMappings {
		for tagValue, bcID := range hierarchy.Mappings {
			rule := RBDRule{
				Condition: RBDCondition{
					Type:      "dimension_equals",
					Dimension: fmt.Sprintf("tag:%s", hierarchy.TagKey),
					Value:     tagValue,
				},
				Value: RBDValue{
					Text: bcID,
				},
			}
			rules = append(rules, rule)
		}
	}
	
	// Add default rule if configured
	if r.config.DefaultBillingCenter != "" {
		defaultRule := RBDRule{
			Condition: RBDCondition{}, // Empty condition matches everything
			Value: RBDValue{
				Text: r.config.DefaultBillingCenter,
			},
		}
		rules = append(rules, defaultRule)
	}
	
	return rules
}

// RBDRule represents a Flexera rule-based dimension rule
type RBDRule struct {
	Condition RBDCondition `json:"condition"`
	Value     RBDValue     `json:"value"`
}

// RBDCondition represents the condition part of an RBD rule
type RBDCondition struct {
	Type       string         `json:"type,omitempty"`      // "dimension_equals", "and", "or", "not"
	Dimension  string         `json:"dimension,omitempty"`
	Value      string         `json:"value,omitempty"`
	Expression *RBDCondition  `json:"expression,omitempty"` // For "not" operator
	Expressions []RBDCondition `json:"expressions,omitempty"` // For "and"/"or" operators
}

// RBDValue represents the value to assign when a rule matches
type RBDValue struct {
	Text string `json:"text"`
}

// Example configuration structure for billing center mappings
func ExampleBillingCenterConfig() BillingCenterMapping {
	return BillingCenterMapping{
		// Direct tag to billing center mappings
		TagMappings: map[string]string{
			"environment:production":  "bc-prod-123",
			"environment:staging":     "bc-staging-456",
			"environment:development": "bc-dev-789",
			"team:platform":          "bc-platform-abc",
			"team:data":              "bc-data-def",
			"cost-center:1001":       "bc-finance-ghi",
			"cost-center:2001":       "bc-marketing-jkl",
		},
		
		// Hierarchical mappings with priority
		HierarchicalMappings: []TagHierarchy{
			{
				Priority:    1,
				TagKey:      "department",
				Description: "Department-level billing allocation",
				Mappings: map[string]string{
					"engineering": "bc-engineering-main",
					"finance":     "bc-finance-main",
					"marketing":   "bc-marketing-main",
					"operations":  "bc-operations-main",
				},
			},
			{
				Priority:    2,
				TagKey:      "project",
				Description: "Project-specific billing allocation",
				Mappings: map[string]string{
					"project-alpha": "bc-alpha-project",
					"project-beta":  "bc-beta-project",
					"project-gamma": "bc-gamma-project",
				},
			},
			{
				Priority:    3,
				TagKey:      "owner",
				Description: "Owner-based billing allocation",
				Mappings: map[string]string{
					"john.doe":   "bc-johndoe-personal",
					"jane.smith": "bc-janesmith-personal",
				},
			},
		},
		
		// Default billing center for unmatched resources
		DefaultBillingCenter: "bc-unallocated-default",
		
		// Enable automatic discovery of billing centers by name
		EnableAutoDiscovery: true,
	}
}