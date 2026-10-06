package server

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rshade/finfocus-plugin-flexera/internal/flexera"
	"github.com/rshade/finfocus-plugin-flexera/internal/flexeraapi"
	"github.com/rshade/finfocus-plugin-flexera/pkg/version"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	pluginName             = "flexera"
	providerAWS            = "aws"
	providerAzure          = "azure"
	providerGCP            = "gcp"
	currencyUSD            = "USD"
	defaultFlexeraZone     = "nam"
	projectionLookbackDays = 90
	hoursPerDay            = 24
	daysPerMonth           = 30
	trendMinPoints         = 30
	pair                   = 2
	triple                 = 3
	maxSelectDays          = 31
	maxSelectMonths        = 24
	selectLimit            = 1000
	minorDigitsDefault     = 2
	minorDigitsNone        = 0
	minorDigitsThree       = 3
	defaultCostMetric      = "cost_amortized_unblended_adj"
	dimResourceID          = "resource_id"
	dimVendor              = "vendor"
	dimService             = "service"
	dimRegion              = "region"
	currencyCacheTTL       = 15 * time.Minute
	forecastLookbackMonths = 3
)

func supportedResourceType(resourceType string) bool {
	switch resourceType {
	case "aws-ec2", "aws-s3", "aws-rds", "azure-vm", "azure-storage", "gcp-compute", "gcp-storage":
		return true
	default:
		return false
	}
}

// FlexeraServer implements finfocus.v1.CostSourceService.
type FlexeraServer struct {
	pbc.UnimplementedCostSourceServiceServer
	cli              *flexera.Client
	api              flexeraapi.Client
	billingCenterIDs []string
	metric           string
	currency         currencyCache
}

type currencyCache struct {
	mu      sync.Mutex
	code    string
	expires time.Time
}

// NewFlexeraServer returns a CostSource server backed by the Flexera client.
func NewFlexeraServer(cli *flexera.Client) *FlexeraServer {
	return &FlexeraServer{cli: cli}
}

// UseCostAPI attaches the unified Flexera client used by the cost RPCs.
func (s *FlexeraServer) UseCostAPI(api flexeraapi.Client, billingCenterIDs []string, metric string) {
	if s == nil {
		return
	}
	s.api = api
	s.billingCenterIDs = append([]string{}, billingCenterIDs...)
	if metric != "" {
		s.metric = metric
	}
}

// RegisterService registers the CostSource service on grpcServer.
func (s *FlexeraServer) RegisterService(grpcServer *grpc.Server) {
	pbc.RegisterCostSourceServiceServer(grpcServer, s)
}

// Name returns the plugin name.
func (s *FlexeraServer) Name(_ context.Context, _ *pbc.NameRequest) (*pbc.NameResponse, error) {
	return &pbc.NameResponse{Name: pluginName}, nil
}

// GetPluginInfo returns plugin version, spec version, providers, and implemented capabilities.
func (s *FlexeraServer) GetPluginInfo(
	_ context.Context,
	_ *pbc.GetPluginInfoRequest,
) (*pbc.GetPluginInfoResponse, error) {
	return &pbc.GetPluginInfoResponse{
		Name:        pluginName,
		Version:     pluginVersion(),
		SpecVersion: pluginsdk.SpecVersion,
		Providers:   []string{providerAWS, providerAzure, providerGCP},
		Capabilities: []pbc.PluginCapability{
			pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS,
			pbc.PluginCapability_PLUGIN_CAPABILITY_PROJECTED_COSTS,
			pbc.PluginCapability_PLUGIN_CAPABILITY_PRICING_SPEC,
		},
		Metadata: map[string]string{
			"implemented_rpcs": "Name,GetPluginInfo,Supports,GetActualCost,GetProjectedCost,GetPricingSpec",
			"flexera_regions":  "nam,eu,apac",
		},
	}, nil
}

// Supports reports whether the plugin can price the requested resource.
func (s *FlexeraServer) Supports(_ context.Context, req *pbc.SupportsRequest) (*pbc.SupportsResponse, error) {
	if req == nil || req.GetResource() == nil {
		return &pbc.SupportsResponse{Supported: false, Reason: "resource descriptor is required"}, nil
	}
	resource := req.GetResource()
	resourceType := strings.ToLower(strings.TrimSpace(resource.GetResourceType()))
	if !supportedResourceType(resourceType) {
		return &pbc.SupportsResponse{
			Supported: false,
			Reason:    fmt.Sprintf("unsupported resource type %q", resource.GetResourceType()),
		}, nil
	}
	if !providerMatches(resourceType, resource.GetProvider()) {
		return &pbc.SupportsResponse{
			Supported: false,
			Reason: fmt.Sprintf(
				"provider %q does not match resource type %q",
				resource.GetProvider(),
				resource.GetResourceType(),
			),
		}, nil
	}
	if reason := s.regionMismatch(resource.GetRegion()); reason != "" {
		return &pbc.SupportsResponse{Supported: false, Reason: reason}, nil
	}
	return &pbc.SupportsResponse{Supported: true}, nil
}

// GetActualCost queries historical cost through the unified Flexera client.
func (s *FlexeraServer) GetActualCost(
	ctx context.Context,
	q *pbc.GetActualCostRequest,
) (*pbc.GetActualCostResponse, error) {
	if q == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	resourceID := q.GetResourceId()
	if resourceID == "" {
		resourceID = q.GetArn()
	}
	if strings.TrimSpace(resourceID) == "" {
		return nil, status.Error(codes.InvalidArgument, "resource_id or arn is required")
	}
	if q.GetStart() == nil || q.GetEnd() == nil {
		return nil, status.Error(codes.InvalidArgument, "start and end are required")
	}
	start := q.GetStart().AsTime()
	end := q.GetEnd().AsTime()
	if !start.Before(end) {
		return nil, status.Error(codes.InvalidArgument, "start must be before end")
	}
	if end.After(start.AddDate(0, maxSelectMonths, 0)) {
		return nil, status.Error(codes.InvalidArgument, "range exceeds 24 months")
	}
	windows, err := chunkDayWindows(start, end)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "cost window: %v", err)
	}
	filter, err := costFilter(resourceID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "resource id: %v", err)
	}
	currency, err := s.currencyCode(ctx)
	if err != nil {
		return nil, err
	}
	if s.api == nil {
		return nil, status.Error(codes.FailedPrecondition, "flexera api client is not configured")
	}
	if len(s.billingCenterIDs) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "billing_center_ids is required")
	}

	rows, err := s.selectCosts(ctx, windows, filter, s.costMetric())
	if err != nil {
		return nil, err
	}
	out := &pbc.GetActualCostResponse{}
	for _, row := range rows {
		amount, ok := row.Metrics[s.costMetric()]
		if !ok {
			continue
		}
		out.Results = append(out.Results, &pbc.ActualCostResult{
			Timestamp: timestamppb.New(row.Timestamp),
			Cost:      roundCurrency(amount, currency),
			Source:    pluginName,
		})
	}
	sortCostResults(out.GetResults())
	return out, nil
}

func (s *FlexeraServer) selectCosts(
	ctx context.Context,
	windows []dayWindow,
	filter *flexeraapi.FilterExpression,
	metric string,
) ([]flexeraapi.CostRow, error) {
	granularity := "day"
	var rows []flexeraapi.CostRow
	for _, window := range windows {
		resp, err := s.api.CostsSelect(ctx, flexeraapi.CostsSelectRequest{
			BillingCenterIDs: s.billingCenterIDs,
			Metrics:          []string{metric},
			Dimensions:       []string{dimResourceID, dimVendor, dimService, dimRegion},
			StartAt:          window.Start.Format(time.DateOnly),
			EndAt:            window.End.Format(time.DateOnly),
			Limit:            selectLimit,
			Granularity:      &granularity,
			Filter:           filter,
		})
		if err != nil {
			return nil, status.Errorf(codes.Unavailable, "flexera costs/select: %v", err)
		}
		if resp.RowsTruncated {
			return nil, status.Error(codes.ResourceExhausted, "flexera costs/select truncated the result")
		}
		rows = append(rows, resp.Rows...)
	}
	return rows, nil
}

// GetProjectedCost estimates a monthly cost from recent costs/select rows.
func (s *FlexeraServer) GetProjectedCost(
	ctx context.Context,
	req *pbc.GetProjectedCostRequest,
) (*pbc.GetProjectedCostResponse, error) {
	if req == nil || req.GetResource() == nil {
		return nil, status.Error(codes.InvalidArgument, "resource descriptor is required")
	}
	resourceType := strings.ToLower(strings.TrimSpace(req.GetResource().GetResourceType()))
	if !supportedResourceType(resourceType) {
		return &pbc.GetProjectedCostResponse{
			Currency:      currencyUSD,
			BillingDetail: "unsupported resource type; projection is 0",
		}, nil
	}
	resourceID := mapResourceDescriptorToID(req.GetResource())
	filter, filterErr := costFilter(resourceID)
	if filterErr == nil {
		if forecasted, ok, forecastErr := s.forecastMonthly(ctx, filter); forecastErr == nil && ok {
			currency, currencyErr := s.currencyCode(ctx)
			if currencyErr != nil {
				return nil, currencyErr
			}
			monthly := roundCurrency(forecasted, currency)
			return &pbc.GetProjectedCostResponse{
				UnitPrice:     monthly / daysPerMonth,
				Currency:      currency,
				CostPerMonth:  monthly,
				BillingDetail: "sum of forecasts/report forecastAmounts for the current month. Estimates only.",
			}, nil
		}
	}
	end := time.Now().UTC()
	start := end.Add(-time.Duration(projectionLookbackDays*hoursPerDay) * time.Hour)
	actual, err := s.GetActualCost(ctx, &pbc.GetActualCostRequest{
		ResourceId: mapResourceDescriptorToID(req.GetResource()),
		Start:      timestamppb.New(start),
		End:        timestamppb.New(end),
	})
	if err != nil {
		return nil, err
	}
	currency := currencyUSD
	if code, currencyErr := s.currencyCode(ctx); currencyErr == nil && code != "" {
		currency = code
	}
	if len(actual.GetResults()) == 0 {
		return &pbc.GetProjectedCostResponse{
			Currency:      currency,
			BillingDetail: "no historical cost rows; projection is 0",
		}, nil
	}
	dailyAverage, monthly := projectMonthly(actual.GetResults(), currency)
	return &pbc.GetProjectedCostResponse{
		UnitPrice:    dailyAverage,
		Currency:     currency,
		CostPerMonth: monthly,
		BillingDetail: "forecasts/report returned no amounts; linear extrapolation of daily " +
			s.costMetric() + " over the lookback window. Estimates only.",
	}, nil
}

// GetPricingSpec returns pricing metadata for a supported resource type.
// Flexera exposes billed cost, not a unit rate card, so rate_per_unit stays 0.
func (s *FlexeraServer) GetPricingSpec(
	ctx context.Context,
	req *pbc.GetPricingSpecRequest,
) (*pbc.GetPricingSpecResponse, error) {
	if req == nil || req.GetResource() == nil {
		return nil, status.Error(codes.InvalidArgument, "resource descriptor is required")
	}
	resource := req.GetResource()
	resourceType := strings.ToLower(strings.TrimSpace(resource.GetResourceType()))
	if !supportedResourceType(resourceType) {
		return nil, status.Errorf(codes.NotFound, "unsupported resource type %q", resource.GetResourceType())
	}
	provider, service, region := parseResourceType(resource.GetResourceType())
	if resource.GetRegion() != "" {
		region = resource.GetRegion()
	}
	currency := currencyUSD
	if code, err := s.currencyCode(ctx); err == nil && code != "" {
		currency = code
	}
	return &pbc.GetPricingSpecResponse{
		Spec: &pbc.PricingSpec{
			Provider:     provider,
			ResourceType: resource.GetResourceType(),
			Region:       region,
			BillingMode:  "consumption",
			Currency:     currency,
			Description:  fmt.Sprintf("Flexera billed cost for %s", resource.GetResourceType()),
			Source:       pluginName,
			Assumptions: []string{
				"Flexera reports billed cost, not a public rate card.",
				"rate_per_unit is 0 because unit price is not available from costs/select.",
				"Use GetActualCost for historical billed cost.",
			},
			PluginMetadata: map[string]string{
				"source":   pluginName,
				"provider": provider,
				"service":  service,
			},
		},
	}, nil
}

func pluginVersion() string {
	v := version.Version
	if v == "" {
		return "v0.0.0"
	}
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

func (s *FlexeraServer) regionMismatch(resourceRegion string) string {
	requested := normalizeFlexeraZone(resourceRegion)
	if requested == "" {
		return ""
	}
	configured := defaultFlexeraZone
	if s != nil && s.cli != nil {
		if zone := normalizeFlexeraZone(s.cli.Region()); zone != "" {
			configured = zone
		}
	}
	if requested == configured {
		return ""
	}
	return fmt.Sprintf(
		"flexera region %q is not served by this plugin (configured for %q)",
		requested,
		configured,
	)
}

func normalizeFlexeraZone(region string) string {
	switch strings.ToLower(strings.TrimSpace(region)) {
	case defaultFlexeraZone, "north-america":
		return defaultFlexeraZone
	case "eu", "europe":
		return "eu"
	case "apac", "asia-pacific":
		return "apac"
	default:
		return ""
	}
}

func providerMatches(resourceType, provider string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return true
	}
	prefix := strings.SplitN(resourceType, "-", pair)[0]
	switch provider {
	case providerAWS, "amazon":
		return prefix == providerAWS
	case providerAzure, "azure-native", "azurerm":
		return prefix == providerAzure
	case providerGCP, "google", "google-native":
		return prefix == providerGCP
	default:
		return false
	}
}

func parseResourceType(resourceType string) (string, string, string) {
	parts := strings.Split(resourceType, "-")
	provider := "unknown"
	if len(parts) >= 1 {
		switch parts[0] {
		case providerAWS:
			provider = "AWS"
		case providerAzure:
			provider = "Azure"
		case providerGCP:
			provider = "GCP"
		}
	}
	service := ""
	if len(parts) >= pair {
		service = strings.Join(parts[1:], "-")
	}
	return provider, service, "global"
}

func mapResourceDescriptorToID(r *pbc.ResourceDescriptor) string {
	if r == nil {
		return ""
	}
	parts := strings.Split(r.GetResourceType(), "-")
	if len(parts) >= 1 {
		switch parts[0] {
		case providerAWS, providerAzure, providerGCP:
			return fmt.Sprintf("service/%s/%s", parts[0], strings.Join(parts[1:], "-"))
		case "cloud":
			if len(parts) >= pair && parts[1] == "account" {
				return "vendor_account/*"
			}
		}
	}
	return ""
}

var _ pbc.CostSourceServiceServer = (*FlexeraServer)(nil)
