package server

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/rshade/finfocus-plugin-flexera/internal/flexeraapi"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type billingFixture struct {
	Rows []struct {
		ID      string `json:"id"`
		Literal string `json:"literal"`
		Vendor  string `json:"vendor"`
		Service string `json:"service"`
	} `json:"rows"`
}

func loadBillingFixture(t *testing.T) billingFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/flexera-dimension-values.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture billingFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if len(fixture.Rows) == 0 {
		t.Fatal("fixture has no rows")
	}
	return fixture
}

func TestBillingForTypeUsesFlexeraDimensionFixture(t *testing.T) {
	fixture := loadBillingFixture(t)
	for _, row := range fixture.Rows {
		t.Run(row.ID, func(t *testing.T) {
			got, ok := billingForType(row.ID)
			if !ok {
				t.Fatalf("billingForType(%q) not found", row.ID)
			}
			if got.Literal != row.Literal || got.Vendor != row.Vendor || got.Service != row.Service {
				t.Fatalf("got %+v, fixture literal %q vendor %q service %q", got, row.Literal, row.Vendor, row.Service)
			}
		})
	}
}

func TestSupportsPulumiTokens(t *testing.T) {
	srv := newTestServer(t, "nam")
	fixture := loadBillingFixture(t)
	for _, row := range fixture.Rows {
		t.Run(row.ID, func(t *testing.T) {
			result, err := srv.Supports(context.Background(), &pbc.SupportsRequest{
				Resource: &pbc.ResourceDescriptor{ResourceType: row.ID, Region: "us-east-1"},
			})
			if err != nil {
				t.Fatalf("Supports: %v", err)
			}
			if !result.GetSupported() {
				t.Fatalf("supported = false, reason %q", result.GetReason())
			}
		})
	}
}

func TestSupportsDeclinesUnknownPulumiToken(t *testing.T) {
	const token = "aws:iam/role:Role"
	result, err := newTestServer(t, "nam").Supports(context.Background(), &pbc.SupportsRequest{
		Resource: &pbc.ResourceDescriptor{Provider: "aws", ResourceType: token},
	})
	if err != nil {
		t.Fatalf("Supports: %v", err)
	}
	if result.GetSupported() {
		t.Fatal("unknown token was accepted")
	}
	if !strings.Contains(result.GetReason(), token) {
		t.Fatalf("reason = %q", result.GetReason())
	}
}

func TestSupportsDeclinesProviderMismatchForToken(t *testing.T) {
	result, err := newTestServer(t, "nam").Supports(context.Background(), &pbc.SupportsRequest{
		Resource: &pbc.ResourceDescriptor{
			Provider:     "azure",
			ResourceType: "aws:ec2/instance:Instance",
		},
	})
	if err != nil {
		t.Fatalf("Supports: %v", err)
	}
	if result.GetSupported() {
		t.Fatal("provider mismatch was accepted")
	}
}

func TestGetPricingSpecAcceptsPulumiToken(t *testing.T) {
	resp, err := newTestServer(t, "nam").GetPricingSpec(context.Background(), &pbc.GetPricingSpecRequest{
		Resource: &pbc.ResourceDescriptor{
			ResourceType: "aws:ec2/instance:Instance",
			Region:       "us-east-1",
		},
	})
	if err != nil {
		t.Fatalf("GetPricingSpec: %v", err)
	}
	if resp.GetSpec().GetResourceType() != "aws:ec2/instance:Instance" {
		t.Fatalf("spec = %+v", resp.GetSpec())
	}
}

func TestGetPricingSpecDeclinesUnknownToken(t *testing.T) {
	_, err := newTestServer(t, "nam").GetPricingSpec(context.Background(), &pbc.GetPricingSpecRequest{
		Resource: &pbc.ResourceDescriptor{ResourceType: "aws:iam/role:Role"},
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %s, err = %v", status.Code(err), err)
	}
}

func TestProjectedCostFilterUsesFlexeraBillingValues(t *testing.T) {
	fixture := loadBillingFixture(t)
	var row struct {
		ID      string
		Literal string
		Vendor  string
		Service string
	}
	for _, item := range fixture.Rows {
		if item.ID == "aws:ec2/instance:Instance" {
			row.ID = item.ID
			row.Vendor = item.Vendor
			row.Service = item.Service
		}
	}
	if row.ID == "" {
		t.Fatal("fixture is missing aws:ec2/instance:Instance")
	}
	api := &fakeAPI{currency: "USD", forecastAmounts: []float64{12}}
	srv := newTestServer(t, "nam")
	srv.UseCostAPI(api, []string{"bc-1"}, "")
	if _, err := srv.GetProjectedCost(context.Background(), &pbc.GetProjectedCostRequest{
		Resource: &pbc.ResourceDescriptor{ResourceType: row.ID},
	}); err != nil {
		t.Fatalf("GetProjectedCost: %v", err)
	}
	if len(api.forecastRequests) != 1 {
		t.Fatalf("forecast calls = %d", len(api.forecastRequests))
	}
	got := equalFilterValues(api.forecastRequests[0].Filter)
	if got["vendor"] != row.Vendor || got["service"] != row.Service {
		t.Fatalf("filter = %v, fixture vendor %q service %q", got, row.Vendor, row.Service)
	}
}

func equalFilterValues(filter *flexeraapi.FilterExpression) map[string]string {
	out := map[string]string{}
	if filter == nil {
		return out
	}
	if filter.Type == "equal" && filter.Dimension != nil && filter.Value != nil {
		out[*filter.Dimension] = *filter.Value
	}
	if filter.Expressions != nil {
		for i := range *filter.Expressions {
			for key, value := range equalFilterValues(&(*filter.Expressions)[i]) {
				out[key] = value
			}
		}
	}
	return out
}
