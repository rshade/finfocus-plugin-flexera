package server

import (
	"context"
	"testing"
	"time"

	unified "github.com/flexera-public/unified-go-client"
	"github.com/rshade/finfocus-plugin-flexera/internal/flexeraapi"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeAPI struct {
	currency            string
	currencyErr         error
	currencyCalls       int
	calls               []flexeraapi.CostsSelectRequest
	rows                []flexeraapi.CostRow
	truncated           bool
	err                 error
	forecastAmounts     []float64
	forecastErr         error
	forecastRequests    []flexeraapi.ForecastRequest
	recommendations     []unified.OptimaRecommendationsRecommendationResultResponse
	recommendationErr   error
	recommendationCalls int
	recommendationQuery *unified.OptimaRecommendationsRecommendationsIndexParams
	statusUpdates       []unified.OptimaRecommendationsRecommendationsUpdateStatusRequestBody
	statusUpdateErr     error
	budgetList          *unified.BudgetBudgetList
	budgetReports       map[string]*unified.BudgetBudgetReportRowList
	budgetByWindow      map[string]*unified.BudgetBudgetReportRowList
	budgetWindowMiss    *unified.BudgetBudgetReportRowList
	budgetShows         map[string]*unified.BudgetBudget
	budgetParams        []*unified.BudgetBudgetReportParams
	budgetErr           error
	budgetOps           []string
	budgetShowIDs       []string
}

func (f *fakeAPI) CostsSelect(
	_ context.Context,
	req flexeraapi.CostsSelectRequest,
) (*flexeraapi.CostsSelectResponse, error) {
	f.calls = append(f.calls, req)
	if f.err != nil {
		return nil, f.err
	}
	return &flexeraapi.CostsSelectResponse{Rows: f.rows, RowsTruncated: f.truncated}, nil
}

func (f *fakeAPI) ForecastReport(
	_ context.Context,
	req flexeraapi.ForecastRequest,
) (*flexeraapi.ForecastResponse, error) {
	f.forecastRequests = append(f.forecastRequests, req)
	if f.forecastErr != nil {
		return nil, f.forecastErr
	}
	if len(f.forecastAmounts) == 0 {
		return &flexeraapi.ForecastResponse{}, nil
	}
	return &flexeraapi.ForecastResponse{
		Segments: []flexeraapi.ForecastSegment{{ForecastAmounts: append([]float64{}, f.forecastAmounts...)}},
	}, nil
}

func (f *fakeAPI) RecommendationsIndex(
	_ context.Context,
	params *unified.OptimaRecommendationsRecommendationsIndexParams,
) ([]unified.OptimaRecommendationsRecommendationResultResponse, error) {
	f.recommendationCalls++
	f.recommendationQuery = params
	if f.recommendationErr != nil {
		return nil, f.recommendationErr
	}
	return append([]unified.OptimaRecommendationsRecommendationResultResponse{}, f.recommendations...), nil
}

func (f *fakeAPI) UpdateRecommendationStatus(
	_ context.Context,
	body unified.OptimaRecommendationsRecommendationsUpdateStatusRequestBody,
) error {
	f.statusUpdates = append(f.statusUpdates, body)
	return f.statusUpdateErr
}

func (f *fakeAPI) BudgetIndex(context.Context) (*unified.BudgetBudgetList, error) {
	f.budgetOps = append(f.budgetOps, "BudgetBudgetIndex")
	if f.budgetErr != nil {
		return nil, f.budgetErr
	}
	return f.budgetList, nil
}

func (f *fakeAPI) BudgetShow(_ context.Context, id string) (*unified.BudgetBudget, error) {
	f.budgetOps = append(f.budgetOps, "BudgetBudgetShow")
	f.budgetShowIDs = append(f.budgetShowIDs, id)
	if f.budgetErr != nil {
		return nil, f.budgetErr
	}
	if f.budgetShows == nil {
		return &unified.BudgetBudget{}, nil
	}
	shown := f.budgetShows[id]
	if shown == nil {
		return &unified.BudgetBudget{}, nil
	}
	return shown, nil
}

func (f *fakeAPI) BudgetReport(
	_ context.Context,
	id string,
	params *unified.BudgetBudgetReportParams,
) (*unified.BudgetBudgetReportRowList, error) {
	f.budgetOps = append(f.budgetOps, "BudgetBudgetReport")
	if params != nil {
		copied := *params
		f.budgetParams = append(f.budgetParams, &copied)
	}
	if f.budgetErr != nil {
		return nil, f.budgetErr
	}
	if params != nil && f.budgetByWindow != nil {
		if report, ok := f.budgetByWindow[params.StartAt]; ok {
			return report, nil
		}
		if f.budgetWindowMiss != nil {
			return f.budgetWindowMiss, nil
		}
	}
	if f.budgetReports == nil {
		return &unified.BudgetBudgetReportRowList{}, nil
	}
	if report := f.budgetReports[id]; report != nil {
		return report, nil
	}
	return &unified.BudgetBudgetReportRowList{}, nil
}

func (f *fakeAPI) CurrencyCode(context.Context) (string, error) {
	f.currencyCalls++
	if f.currencyErr != nil {
		return "", f.currencyErr
	}
	if f.currency == "" {
		return "USD", nil
	}
	return f.currency, nil
}

func TestGetActualCostUsesCostsSelect(t *testing.T) {
	api := &fakeAPI{
		currency: "USD",
		rows: []flexeraapi.CostRow{{
			Timestamp: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
			Metrics:   map[string]float64{"cost_amortized_unblended_adj": 1.235},
		}},
	}
	srv := newTestServer(t, "nam")
	srv.UseCostAPI(api, []string{"bc-1"}, "")

	resp, err := srv.GetActualCost(context.Background(), &pbc.GetActualCostRequest{
		ResourceId: "i-abc",
		Start:      timestamppb.New(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		End:        timestamppb.New(time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		t.Fatalf("GetActualCost: %v", err)
	}
	if len(api.calls) != 1 {
		t.Fatalf("calls = %d", len(api.calls))
	}
	call := api.calls[0]
	if call.BillingCenterIDs[0] != "bc-1" || call.Metrics[0] != "cost_amortized_unblended_adj" {
		t.Fatalf("request = %+v", call)
	}
	if call.Filter == nil || call.Filter.Type != "equal" || call.Filter.Value == nil || *call.Filter.Value != "i-abc" {
		t.Fatalf("filter = %+v", call.Filter)
	}
	if len(resp.GetResults()) != 1 || resp.GetResults()[0].GetCost() != 1.24 {
		t.Fatalf("results = %+v", resp.GetResults())
	}

	_, secondErr := srv.GetActualCost(context.Background(), &pbc.GetActualCostRequest{
		ResourceId: "i-abc",
		Start:      timestamppb.New(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		End:        timestamppb.New(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)),
	})
	if secondErr != nil {
		t.Fatalf("second GetActualCost: %v", secondErr)
	}
	if api.currencyCalls != 1 {
		t.Fatalf("currency calls = %d, want 1", api.currencyCalls)
	}
}

func TestGetActualCostChunksDayWindows(t *testing.T) {
	api := &fakeAPI{currency: "USD"}
	srv := newTestServer(t, "nam")
	srv.UseCostAPI(api, []string{"bc-1"}, "")
	_, err := srv.GetActualCost(context.Background(), &pbc.GetActualCostRequest{
		ResourceId: "arn:aws:ec2:us-east-1:1:instance/i-1",
		Start:      timestamppb.New(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		End:        timestamppb.New(time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		t.Fatalf("GetActualCost: %v", err)
	}
	if len(api.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(api.calls))
	}
	if api.calls[0].StartAt != "2026-01-01" || api.calls[0].EndAt != "2026-02-01" {
		t.Fatalf("first window %s %s", api.calls[0].StartAt, api.calls[0].EndAt)
	}
	if api.calls[1].StartAt != "2026-02-01" || api.calls[1].EndAt != "2026-02-10" {
		t.Fatalf("second window %s %s", api.calls[1].StartAt, api.calls[1].EndAt)
	}
}

func TestGetActualCostRequiresBillingCenters(t *testing.T) {
	srv := newTestServer(t, "nam")
	srv.UseCostAPI(&fakeAPI{}, nil, "")
	_, err := srv.GetActualCost(context.Background(), &pbc.GetActualCostRequest{
		ResourceId: "i-abc",
		Start:      timestamppb.New(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		End:        timestamppb.New(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)),
	})
	if err == nil {
		t.Fatal("expected billing center error")
	}
}

func TestGetProjectedCostExtrapolates(t *testing.T) {
	api := &fakeAPI{
		currency: "USD",
		rows: []flexeraapi.CostRow{
			{
				Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				Metrics:   map[string]float64{defaultCostMetric: 10},
			},
			{
				Timestamp: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
				Metrics:   map[string]float64{defaultCostMetric: 20},
			},
		},
	}
	srv := newTestServer(t, "nam")
	srv.UseCostAPI(api, []string{"bc-1"}, "")
	resp, err := srv.GetProjectedCost(context.Background(), &pbc.GetProjectedCostRequest{
		Resource: &pbc.ResourceDescriptor{ResourceType: "aws-ec2"},
	})
	if err != nil {
		t.Fatalf("GetProjectedCost: %v", err)
	}
	if resp.GetCostPerMonth() != 450 || resp.GetUnitPrice() != 15 {
		t.Fatalf("projection = %+v", resp)
	}
	if resp.GetBillingDetail() == "" {
		t.Fatal("expected methodology in billing_detail")
	}
	if api.calls[0].Filter == nil || api.calls[0].Filter.Type != "and" {
		t.Fatalf("expected service filter, got %+v", api.calls[0].Filter)
	}
}

func TestGetProjectedCostUsesForecastReport(t *testing.T) {
	api := &fakeAPI{currency: "USD", forecastAmounts: []float64{10, 20.5}}
	srv := newTestServer(t, "nam")
	srv.UseCostAPI(api, []string{"bc-1"}, "")
	resp, err := srv.GetProjectedCost(context.Background(), &pbc.GetProjectedCostRequest{
		Resource: &pbc.ResourceDescriptor{ResourceType: "aws-ec2"},
	})
	if err != nil {
		t.Fatalf("GetProjectedCost: %v", err)
	}
	if resp.GetCostPerMonth() != 30.5 || len(api.calls) != 0 {
		t.Fatalf("projection = %+v calls = %d", resp, len(api.calls))
	}
	if len(api.forecastRequests) != 1 || api.forecastRequests[0].Granularity != "month" {
		t.Fatalf("forecast request = %+v", api.forecastRequests)
	}
	if api.forecastRequests[0].Metric != defaultCostMetric ||
		api.forecastRequests[0].LookbackPeriod != forecastLookbackMonths {
		t.Fatalf("forecast request = %+v", api.forecastRequests[0])
	}
}

func TestGetProjectedCostUnsupported(t *testing.T) {
	srv := newTestServer(t, "nam")
	resp, err := srv.GetProjectedCost(context.Background(), &pbc.GetProjectedCostRequest{
		Resource: &pbc.ResourceDescriptor{ResourceType: "k8s-pod"},
	})
	if err != nil {
		t.Fatalf("GetProjectedCost: %v", err)
	}
	if resp.GetCostPerMonth() != 0 {
		t.Fatalf("cost = %v", resp.GetCostPerMonth())
	}
}

func TestRoundCurrency(t *testing.T) {
	if got := roundCurrency(1.235, "USD"); got != 1.24 {
		t.Fatalf("USD = %v", got)
	}
	if got := roundCurrency(1.6, "JPY"); got != 2 {
		t.Fatalf("JPY = %v", got)
	}
}

func TestCostFilterRejectsBillingCenter(t *testing.T) {
	if _, err := costFilter("billing_center/bc-1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestGetActualCostRejectsLongRange(t *testing.T) {
	srv := newTestServer(t, "nam")
	srv.UseCostAPI(&fakeAPI{}, []string{"bc-1"}, "")
	_, err := srv.GetActualCost(context.Background(), &pbc.GetActualCostRequest{
		ResourceId: "i-abc",
		Start:      timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
		End:        timestamppb.New(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
	})
	if err == nil {
		t.Fatal("expected a 24 month limit error")
	}
}

func TestGetActualCostTruncated(t *testing.T) {
	srv := newTestServer(t, "nam")
	srv.UseCostAPI(&fakeAPI{truncated: true}, []string{"bc-1"}, "")
	_, err := srv.GetActualCost(context.Background(), &pbc.GetActualCostRequest{
		ResourceId: "i-abc",
		Start:      timestamppb.New(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		End:        timestamppb.New(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)),
	})
	if err == nil {
		t.Fatal("expected truncation error")
	}
}

func TestGetActualCostAPIError(t *testing.T) {
	srv := newTestServer(t, "nam")
	srv.UseCostAPI(&fakeAPI{err: context.DeadlineExceeded}, []string{"bc-1"}, "")
	_, err := srv.GetActualCost(context.Background(), &pbc.GetActualCostRequest{
		ResourceId: "i-abc",
		Start:      timestamppb.New(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		End:        timestamppb.New(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)),
	})
	if err == nil {
		t.Fatal("expected API error")
	}
}

func TestGetPricingSpecUsesCurrency(t *testing.T) {
	api := &fakeAPI{currency: "EUR"}
	srv := newTestServer(t, "nam")
	srv.UseCostAPI(api, []string{"bc-1"}, "")
	resp, err := srv.GetPricingSpec(context.Background(), &pbc.GetPricingSpecRequest{
		Resource: &pbc.ResourceDescriptor{ResourceType: "aws-ec2", Region: "us-east-1"},
	})
	if err != nil {
		t.Fatalf("GetPricingSpec: %v", err)
	}
	spec := resp.GetSpec()
	if spec.GetProvider() != "AWS" || spec.GetCurrency() != "EUR" || spec.GetRatePerUnit() != 0 {
		t.Fatalf("spec = %+v", spec)
	}
	if len(spec.GetAssumptions()) == 0 {
		t.Fatal("expected billing model assumptions")
	}
}

func TestGetPricingSpecUnsupported(t *testing.T) {
	srv := newTestServer(t, "nam")
	_, err := srv.GetPricingSpec(context.Background(), &pbc.GetPricingSpecRequest{
		Resource: &pbc.ResourceDescriptor{ResourceType: "k8s-pod"},
	})
	if err == nil {
		t.Fatal("expected unsupported resource error")
	}
}
