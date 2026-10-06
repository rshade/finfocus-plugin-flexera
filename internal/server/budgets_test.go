package server

import (
	"context"
	"strings"
	"testing"
	"time"

	unified "github.com/flexera-public/unified-go-client"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGetBudgetsReadsIndexAndReport(t *testing.T) {
	reportAmount := 125.5
	spend := 40.25
	now := time.Now().UTC()
	reportRow := unified.BudgetBudgetReportRow{
		Timestamp: time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC),
		Metrics: unified.BudgetBudgetMetrics{
			BudgetAmount: &reportAmount,
			SpendAmount:  &spend,
		},
	}
	fromReport := unified.BudgetBudgetLink{Id: "bud-report", Name: "Platform"}
	segment := unified.BudgetBudgetSegment{BudgetAmounts: []float64{80}}
	fromShow := unified.BudgetBudget{
		Id:       "bud-show",
		Name:     "Fallback",
		Segments: []unified.BudgetBudgetSegment{segment},
	}
	showLink := unified.BudgetBudgetLink{Id: fromShow.Id, Name: fromShow.Name}
	showSpend := 10.0
	api := &fakeAPI{
		budgetList: &unified.BudgetBudgetList{Values: []unified.BudgetBudgetLink{fromReport, showLink}},
		budgetReports: map[string]*unified.BudgetBudgetReportRowList{
			fromReport.Id: {Values: []unified.BudgetBudgetReportRow{reportRow}},
			fromShow.Id: {Values: []unified.BudgetBudgetReportRow{{
				Metrics: unified.BudgetBudgetMetrics{SpendAmount: &showSpend},
			}}},
		},
		budgetShows: map[string]*unified.BudgetBudget{fromShow.Id: &fromShow},
	}
	srv := newTestServer(t, "nam")
	srv.UseCostAPI(api, nil, "")

	without, err := srv.GetBudgets(context.Background(), &pbc.GetBudgetsRequest{})
	if err != nil {
		t.Fatalf("GetBudgets: %v", err)
	}
	reportBudget := budgetByID(t, without, fromReport.Id)
	if reportBudget.GetName() != fromReport.Name {
		t.Fatalf("name = %q, source name = %q", reportBudget.GetName(), fromReport.Name)
	}
	gotLimit := reportBudget.GetAmount().GetLimit()
	if gotLimit != *reportRow.Metrics.BudgetAmount {
		t.Fatalf("limit = %v, source budgetAmount = %v", gotLimit, *reportRow.Metrics.BudgetAmount)
	}
	if reportBudget.GetStatus() != nil {
		t.Fatalf("status = %+v", reportBudget.GetStatus())
	}
	showBudget := budgetByID(t, without, fromShow.Id)
	wantSegment := segment.BudgetAmounts[len(segment.BudgetAmounts)-1]
	if showBudget.GetAmount().GetLimit() != wantSegment {
		t.Fatalf("limit = %v, source budgetAmounts = %v", showBudget.GetAmount().GetLimit(), wantSegment)
	}
	if showBudget.GetName() != fromShow.Name || showBudget.GetStatus() != nil {
		t.Fatalf("show budget = %+v", showBudget)
	}
	assertReadOnlyBudgetOps(t, api.budgetOps)
	if len(api.budgetShowIDs) != 1 || api.budgetShowIDs[0] != fromShow.Id {
		t.Fatalf("show ids = %v", api.budgetShowIDs)
	}

	api.budgetOps = nil
	withStatus, err := srv.GetBudgets(context.Background(), &pbc.GetBudgetsRequest{IncludeStatus: true})
	if err != nil {
		t.Fatalf("include status: %v", err)
	}
	got := budgetByID(t, withStatus, fromReport.Id)
	gotSpend := got.GetStatus().GetCurrentSpend()
	if gotSpend != *reportRow.Metrics.SpendAmount {
		t.Fatalf("spend = %v, source spendAmount = %v", gotSpend, *reportRow.Metrics.SpendAmount)
	}
	assertReadOnlyBudgetOps(t, api.budgetOps)
}

func TestGetBudgetsSumsCurrentMonthAcrossSegments(t *testing.T) {
	now := time.Now().UTC()
	current := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	other := current.AddDate(0, -1, 0)
	link := unified.BudgetBudgetLink{
		Id:         "bud-span",
		Name:       "Two months",
		YearMonths: []string{other.Format("2006-01"), current.Format("2006-01")},
	}
	segments := []unified.BudgetBudgetSegment{
		{BudgetAmounts: []float64{1000, 100}},
		{BudgetAmounts: []float64{500, 80}},
	}
	monthIndex := len(link.YearMonths) - 1
	amounts := []float64{
		segments[0].BudgetAmounts[monthIndex],
		segments[1].BudgetAmounts[monthIndex],
	}
	spends := []float64{40, 40}
	var wantLimit float64
	for _, segment := range segments {
		wantLimit += segment.BudgetAmounts[monthIndex]
	}
	currentRows := []unified.BudgetBudgetReportRow{
		{
			Timestamp: current,
			Metrics: unified.BudgetBudgetMetrics{
				BudgetAmount: &amounts[0],
				SpendAmount:  &spends[0],
			},
		},
		{
			Timestamp: current,
			Metrics: unified.BudgetBudgetMetrics{
				BudgetAmount: &amounts[1],
				SpendAmount:  &spends[1],
			},
		},
	}
	var wantSpend float64
	for _, row := range currentRows {
		wantSpend += *row.Metrics.SpendAmount
	}
	fullSpan := segments[0].BudgetAmounts[0] + segments[1].BudgetAmounts[0] + wantLimit
	api := &fakeAPI{
		budgetList: &unified.BudgetBudgetList{Values: []unified.BudgetBudgetLink{link}},
		budgetByWindow: map[string]*unified.BudgetBudgetReportRowList{
			current.Format("2006-01"): {Values: currentRows},
		},
		budgetWindowMiss: &unified.BudgetBudgetReportRowList{
			Values: []unified.BudgetBudgetReportRow{{
				Timestamp: other,
				Metrics: unified.BudgetBudgetMetrics{
					BudgetAmount: &fullSpan,
					SpendAmount:  &fullSpan,
				},
			}},
		},
		budgetShows: map[string]*unified.BudgetBudget{
			link.Id: {Id: link.Id, Name: link.Name, Segments: segments},
		},
	}
	srv := newTestServer(t, "nam")
	srv.UseCostAPI(api, nil, "")

	resp, err := srv.GetBudgets(context.Background(), &pbc.GetBudgetsRequest{IncludeStatus: true})
	if err != nil {
		t.Fatalf("GetBudgets: %v", err)
	}
	got := budgetByID(t, resp, link.Id)
	if got.GetPeriod() != pbc.BudgetPeriod_BUDGET_PERIOD_MONTHLY {
		t.Fatalf("period = %s", got.GetPeriod())
	}
	if got.GetAmount().GetLimit() != wantLimit {
		t.Fatalf("limit = %v, month budgetAmounts sum = %v", got.GetAmount().GetLimit(), wantLimit)
	}
	gotSpend := got.GetStatus().GetCurrentSpend()
	if gotSpend != wantSpend {
		t.Fatalf("spend = %v, month spend sum = %v", gotSpend, wantSpend)
	}
	if len(api.budgetParams) != 1 {
		t.Fatalf("report calls = %d", len(api.budgetParams))
	}
	start := current.Format("2006-01")
	end := current.AddDate(0, 1, 0).Format("2006-01")
	if api.budgetParams[0].StartAt != start || api.budgetParams[0].EndAt != end {
		t.Fatalf("window = %s..%s", api.budgetParams[0].StartAt, api.budgetParams[0].EndAt)
	}
	assertReadOnlyBudgetOps(t, api.budgetOps)
}

func TestGetBudgetsRequiresClient(t *testing.T) {
	_, err := newTestServer(t, "nam").GetBudgets(context.Background(), &pbc.GetBudgetsRequest{})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %s", status.Code(err))
	}
}

func TestGetPluginInfoAdvertisesBudgets(t *testing.T) {
	info, err := newTestServer(t, "nam").GetPluginInfo(context.Background(), &pbc.GetPluginInfoRequest{})
	if err != nil {
		t.Fatalf("GetPluginInfo: %v", err)
	}
	found := false
	for _, cap := range info.GetCapabilities() {
		if cap == pbc.PluginCapability_PLUGIN_CAPABILITY_BUDGETS {
			found = true
		}
	}
	if !found {
		t.Fatal("missing PLUGIN_CAPABILITY_BUDGETS")
	}
	if !rpcListed(info.GetMetadata()["implemented_rpcs"], "GetBudgets") {
		t.Fatalf("implemented_rpcs = %q", info.GetMetadata()["implemented_rpcs"])
	}
}

func budgetByID(t *testing.T, resp *pbc.GetBudgetsResponse, id string) *pbc.Budget {
	t.Helper()
	for _, budget := range resp.GetBudgets() {
		if budget.GetId() == id {
			return budget
		}
	}
	t.Fatalf("missing budget %s", id)
	return nil
}

func assertReadOnlyBudgetOps(t *testing.T, ops []string) {
	t.Helper()
	if len(ops) == 0 {
		t.Fatal("no budget calls")
	}
	sawIndex := false
	sawReport := false
	for _, op := range ops {
		switch op {
		case "BudgetBudgetIndex":
			sawIndex = true
		case "BudgetBudgetReport":
			sawReport = true
		case "BudgetBudgetShow":
		default:
			t.Fatalf("unexpected budget op %s", op)
		}
		lower := strings.ToLower(op)
		if strings.Contains(lower, "create") || strings.Contains(lower, "update") || strings.Contains(lower, "delete") {
			t.Fatalf("write op %s", op)
		}
	}
	if !sawIndex || !sawReport {
		t.Fatalf("ops = %v", ops)
	}
}
