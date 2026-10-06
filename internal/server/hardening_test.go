package server

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/rshade/finfocus-plugin-flexera/internal/flexera"
	"github.com/rshade/finfocus-plugin-flexera/internal/flexeraapi"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type blockingAPI struct {
	fakeAPI
}

func (b *blockingAPI) CostsSelect(
	ctx context.Context,
	req flexeraapi.CostsSelectRequest,
) (*flexeraapi.CostsSelectResponse, error) {
	b.calls = append(b.calls, req)
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b *blockingAPI) ForecastReport(
	ctx context.Context,
	req flexeraapi.ForecastRequest,
) (*flexeraapi.ForecastResponse, error) {
	b.forecastRequests = append(b.forecastRequests, req)
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b *blockingAPI) CurrencyCode(ctx context.Context) (string, error) {
	b.currencyCalls++
	<-ctx.Done()
	return "", ctx.Err()
}

func actualRequest() *pbc.GetActualCostRequest {
	return &pbc.GetActualCostRequest{
		ResourceId: "i-abc",
		Start:      timestamppb.New(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		End:        timestamppb.New(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)),
	}
}

func projectedRequest() *pbc.GetProjectedCostRequest {
	return &pbc.GetProjectedCostRequest{
		Resource: &pbc.ResourceDescriptor{ResourceType: "aws-ec2", Region: "us-east-1"},
	}
}

func pricingRequest() *pbc.GetPricingSpecRequest {
	return &pbc.GetPricingSpecRequest{
		Resource: &pbc.ResourceDescriptor{ResourceType: "aws-ec2", Region: "us-east-1"},
	}
}

func TestCostRPCsHitOwnDeadline(t *testing.T) {
	api := &blockingAPI{}
	srv := newTestServer(t, "nam")
	srv.rpcTimeout = 40 * time.Millisecond
	srv.UseCostAPI(api, []string{"bc-1"}, "")

	started := time.Now()
	_, actualErr := srv.GetActualCost(context.Background(), actualRequest())
	_, projectedErr := srv.GetProjectedCost(context.Background(), projectedRequest())
	_, pricingErr := srv.GetPricingSpec(context.Background(), pricingRequest())
	if time.Since(started) > 2*time.Second {
		t.Fatal("hung dependency blocked past the RPC deadline")
	}
	for _, err := range []error{actualErr, projectedErr, pricingErr} {
		if status.Code(err) != codes.DeadlineExceeded {
			t.Fatalf("code = %s, err = %v", status.Code(err), err)
		}
	}
}

func TestCostRPCsShareHTTPStatus(t *testing.T) {
	failure := &flexeraapi.StatusError{Op: "flexera", Status: http.StatusUnauthorized, Detail: "unauthorized"}
	api := &fakeAPI{currencyErr: failure, err: failure, forecastErr: failure}
	srv := newTestServer(t, "nam")
	srv.UseCostAPI(api, []string{"bc-1"}, "")

	_, actualErr := srv.GetActualCost(context.Background(), actualRequest())
	_, projectedErr := srv.GetProjectedCost(context.Background(), projectedRequest())
	_, pricingErr := srv.GetPricingSpec(context.Background(), pricingRequest())
	for _, err := range []error{actualErr, projectedErr, pricingErr} {
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("code = %s, err = %v", status.Code(err), err)
		}
	}

	unavailable := &flexeraapi.StatusError{Op: "flexera", Status: http.StatusBadGateway, Detail: "down"}
	api.currencyErr = unavailable
	api.err = unavailable
	api.forecastErr = unavailable
	_, actualErr = srv.GetActualCost(context.Background(), actualRequest())
	_, projectedErr = srv.GetProjectedCost(context.Background(), projectedRequest())
	_, pricingErr = srv.GetPricingSpec(context.Background(), pricingRequest())
	for _, err := range []error{actualErr, projectedErr, pricingErr} {
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("code = %s, err = %v", status.Code(err), err)
		}
	}
}

func TestCostRPCCanceledContext(t *testing.T) {
	srv := newTestServer(t, "nam")
	srv.UseCostAPI(&fakeAPI{}, []string{"bc-1"}, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := srv.GetActualCost(ctx, actualRequest())
	if status.Code(err) != codes.Canceled {
		t.Fatalf("code = %s", status.Code(err))
	}
}

func TestGetActualCostEnrichesBillingCenterFromTags(t *testing.T) {
	cfg := flexera.Config{
		OrgID:    "1",
		APIToken: "test-token",
		Region:   "nam",
		BillingCenterMappings: &flexera.BillingCenterMapping{
			TagMappings: map[string]string{"environment:production": "bc-prod-123"},
		},
	}
	cli, err := flexera.NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	api := &fakeAPI{
		currency: "USD",
		rows: []flexeraapi.CostRow{{
			Timestamp:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Dimensions: map[string]string{"tag_environment": "production"},
			Metrics:    map[string]float64{"cost_amortized_unblended_adj": 2},
		}},
	}
	srv := NewFlexeraServer(cli)
	srv.UseCostAPI(api, []string{"bc-1"}, "")
	resp, err := srv.GetActualCost(context.Background(), actualRequest())
	if err != nil {
		t.Fatalf("GetActualCost: %v", err)
	}
	if len(api.calls) != 1 || !hasDimension(api.calls[0].Dimensions, "tag_environment") {
		t.Fatalf("costs/select dimensions = %#v", api.calls[0].Dimensions)
	}
	if len(resp.GetResults()) != 1 {
		t.Fatalf("results = %d", len(resp.GetResults()))
	}
	parent := resp.GetResults()[0].GetLineage().GetParent()
	if parent.GetId() != "bc-prod-123" {
		t.Fatalf("billing center = %q", parent.GetId())
	}
	if parent.GetType() != pbc.LineageNodeType_LINEAGE_NODE_TYPE_BILLING_ACCOUNT {
		t.Fatalf("lineage type = %s", parent.GetType())
	}
}

func TestGetActualCostUsesDefaultBillingCenterWithoutTags(t *testing.T) {
	cfg := flexera.Config{
		OrgID:  "1",
		Region: "nam",
		BillingCenterMappings: &flexera.BillingCenterMapping{
			TagMappings:          map[string]string{"environment:production": "bc-prod-123"},
			DefaultBillingCenter: "bc-unallocated",
		},
	}
	cli, err := flexera.NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	api := &fakeAPI{
		currency: "USD",
		rows: []flexeraapi.CostRow{{
			Timestamp:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Dimensions: map[string]string{"vendor": "aws"},
			Metrics:    map[string]float64{"cost_amortized_unblended_adj": 2},
		}},
	}
	srv := NewFlexeraServer(cli)
	srv.UseCostAPI(api, []string{"bc-1"}, "")
	resp, err := srv.GetActualCost(context.Background(), actualRequest())
	if err != nil {
		t.Fatalf("GetActualCost: %v", err)
	}
	if len(api.calls) != 1 || !hasDimension(api.calls[0].Dimensions, "tag_environment") {
		t.Fatalf("costs/select dimensions = %#v", api.calls[0].Dimensions)
	}
	parent := resp.GetResults()[0].GetLineage().GetParent()
	if parent.GetId() != "bc-unallocated" {
		t.Fatalf("billing center = %q", parent.GetId())
	}
}

func hasDimension(dimensions []string, want string) bool {
	for _, dim := range dimensions {
		if dim == want {
			return true
		}
	}
	return false
}
