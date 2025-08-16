package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rshade/pulumi-plugin-flexera/internal/flexera"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TODO: Replace these stubs when pulumicost-spec protobuf definitions are available
type UnimplementedCostSourceServer struct{}
type Empty struct{}
type PluginName struct{ Name string }
type ResourceDescriptor struct{ ResourceType string }
type SupportsResponse struct{ Supported bool }
type ActualCostQuery struct{ ResourceId, Start, End string }
type ActualCostResult struct{ Timestamp *timestamppb.Timestamp; Cost float64; UsageAmount float64; UsageUnit, Source string }
type ActualCostResultList struct{ Results []*ActualCostResult }
type PriceInfo struct{ UnitPrice, CostPerMonth float64; Currency, BillingDetail string }
type PricingSpec struct{ Provider, ResourceType, Sku, Region, BillingMode, Currency, Description string; RatePerUnit float64; PluginMetadata map[string]string }

type FlexeraServer struct {
	UnimplementedCostSourceServer
	cli *flexera.Client
}

func NewFlexeraServer(cli *flexera.Client) *FlexeraServer {
	return &FlexeraServer{cli: cli}
}

func (s *FlexeraServer) RegisterService(grpcServer *grpc.Server) {
	// TODO: Uncomment when pulumicost-spec is available
	// pbc.RegisterCostSourceServer(grpcServer, s)
}

func (s *FlexeraServer) Name(ctx context.Context, _ *Empty) (*PluginName, error) {
	return &PluginName{Name: "flexera-optima"}, nil
}

func (s *FlexeraServer) Supports(ctx context.Context, r *ResourceDescriptor) (*SupportsResponse, error) {
	// Flexera Optima supports cloud resource types
	supported := strings.HasPrefix(r.ResourceType, "aws-") ||
		strings.HasPrefix(r.ResourceType, "azure-") ||
		strings.HasPrefix(r.ResourceType, "gcp-") ||
		r.ResourceType == "cloud-account" ||
		r.ResourceType == "cloud-service" ||
		r.ResourceType == "cloud-region" ||
		r.ResourceType == "billing-center" ||
		r.ResourceType == "resource-group"
	return &SupportsResponse{Supported: supported}, nil
}

func (s *FlexeraServer) GetActualCost(ctx context.Context, q *ActualCostQuery) (*ActualCostResultList, error) {
	// Build Flexera cost query from the request
	query := s.cli.BuildCostQuery(q.ResourceId, q.Start, q.End)
	
	resp, err := s.cli.Costs(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query flexera costs: %w", err)
	}

	out := &ActualCostResultList{}
	for _, point := range resp.Results {
		// Convert Flexera cost point to ActualCostResult
		timestamp, err := time.Parse("2006-01-02", point.Timestamp)
		if err != nil {
			// Try alternative formats
			if timestamp, err = time.Parse(time.RFC3339, point.Timestamp); err != nil {
				continue // Skip invalid timestamps
			}
		}
		
		// Get the primary cost metric
		cost := 0.0
		if costValue, exists := point.Metrics["cost_amortized_unblended_adj"]; exists {
			cost = costValue
		} else if costValue, exists := point.Metrics["cost"]; exists {
			cost = costValue
		}
		
		acr := &ActualCostResult{
			Timestamp:   timestamppb.New(timestamp),
			Cost:        cost,
			UsageAmount: 0,  // Flexera doesn't typically include usage amounts in cost queries
			UsageUnit:   "", 
			Source:      "flexera-optima",
		}
		out.Results = append(out.Results, acr)
	}
	return out, nil
}

func (s *FlexeraServer) GetProjectedCost(ctx context.Context, r *ResourceDescriptor) (*PriceInfo, error) {
	// For cost projection, analyze last 90 days of data for trends
	end := time.Now().UTC()
	start := end.Add(-90 * 24 * time.Hour) // 90-day lookback for better trend analysis
	
	acr, err := s.GetActualCost(ctx, &ActualCostQuery{
		ResourceId: mapResourceDescriptorToID(r),
		Start:      start.Format(time.RFC3339),
		End:        end.Format(time.RFC3339),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get historical cost data: %w", err)
	}
	
	if len(acr.Results) == 0 {
		return &PriceInfo{Currency: "USD"}, nil
	}

	// Calculate trend-based projection
	var totalCost float64
	for _, p := range acr.Results {
		totalCost += p.Cost
	}
	
	// Calculate daily average and project monthly cost
	dailyAverage := totalCost / float64(len(acr.Results))
	monthlyProjection := dailyAverage * 30.0
	
	// Apply trend analysis if we have enough data points
	if len(acr.Results) >= 30 {
		// Simple trend calculation: compare first and last 15 days
		firstHalfSum := 0.0
		secondHalfSum := 0.0
		midPoint := len(acr.Results) / 2
		
		for i := 0; i < midPoint; i++ {
			firstHalfSum += acr.Results[i].Cost
		}
		for i := midPoint; i < len(acr.Results); i++ {
			secondHalfSum += acr.Results[i].Cost
		}
		
		firstHalfAvg := firstHalfSum / float64(midPoint)
		secondHalfAvg := secondHalfSum / float64(len(acr.Results)-midPoint)
		
		// Apply trend factor to projection
		if firstHalfAvg > 0 {
			trendFactor := secondHalfAvg / firstHalfAvg
			monthlyProjection *= trendFactor
		}
	}

	return &PriceInfo{
		UnitPrice:     dailyAverage,
		Currency:      "USD", // Flexera typically returns USD, but could be configurable
		CostPerMonth:  monthlyProjection,
		BillingDetail: "flexera-trend-projection",
	}, nil
}

func (s *FlexeraServer) GetPricingSpec(ctx context.Context, r *ResourceDescriptor) (*PricingSpec, error) {
	// Extract vendor, service, region from resource type if possible
	provider, service, region := parseResourceType(r.ResourceType)
	
	return &PricingSpec{
		Provider:       provider,
		ResourceType:   r.ResourceType,
		Sku:            "", // Flexera abstracts SKU details
		Region:         region,
		BillingMode:    "consumption", // Flexera tracks actual consumption-based billing
		RatePerUnit:    0, // Rate varies by actual cloud provider pricing
		Currency:       "USD",
		Description:    fmt.Sprintf("Flexera Optima cost tracking for %s", r.ResourceType),
		PluginMetadata: map[string]string{
			"source":   "flexera-optima",
			"provider": provider,
			"service":  service,
		},
	}, nil
}

// parseResourceType extracts provider, service, and region from resource type
func parseResourceType(resourceType string) (provider, service, region string) {
	parts := strings.Split(resourceType, "-")
	if len(parts) >= 1 {
		switch parts[0] {
		case "aws":
			provider = "AWS"
		case "azure":
			provider = "Azure" 
		case "gcp":
			provider = "GCP"
		default:
			provider = "unknown"
		}
	}
	
	if len(parts) >= 2 {
		service = strings.Join(parts[1:], "-")
	}
	
	// Region would need to be extracted from the actual resource ID or context
	region = "global" // Default value
	
	return provider, service, region
}

// mapResourceDescriptorToID converts a ResourceDescriptor to a resource ID for cost queries
func mapResourceDescriptorToID(r *ResourceDescriptor) string {
	// This is a basic mapping - in practice, you'd need more context
	// about which specific resource instance to query
	parts := strings.Split(r.ResourceType, "-")
	if len(parts) >= 1 {
		switch parts[0] {
		case "aws", "azure", "gcp":
			// Return a service-level ID for the cloud provider
			return fmt.Sprintf("service/%s/%s", parts[0], strings.Join(parts[1:], "-"))
		case "cloud":
			if len(parts) >= 2 && parts[1] == "account" {
				return "vendor_account/*" // Wildcard to match all accounts
			}
		}
	}
	
	// Default to empty string which will query all resources
	return ""
}