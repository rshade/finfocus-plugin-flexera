package server

import (
	"context"
	"errors"
	"time"

	unified "github.com/flexera-public/unified-go-client"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	budgetWarnPercent     = 75
	budgetCriticalPercent = 90
	fullPercent           = 100
)

var errNoBudgetAmount = errors.New("budget amount is missing")

// GetBudgets returns read-only budgets from the Flexera budget index and report.
func (s *FlexeraServer) GetBudgets(
	ctx context.Context,
	req *pbc.GetBudgetsRequest,
) (*pbc.GetBudgetsResponse, error) {
	if req == nil {
		req = &pbc.GetBudgetsRequest{}
	}
	ctx, cancel, err := s.beginRPC(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	if s == nil || s.api == nil {
		return nil, status.Error(codes.FailedPrecondition, missingFlexeraClient)
	}
	list, err := s.api.BudgetIndex(ctx)
	if err != nil {
		return nil, mapFailure(err)
	}
	currency, err := s.currencyCode(ctx)
	if err != nil {
		return nil, err
	}
	return s.budgetsFromIndex(ctx, list, req.GetIncludeStatus(), currency)
}

func (s *FlexeraServer) budgetsFromIndex(
	ctx context.Context,
	list *unified.BudgetBudgetList,
	includeStatus bool,
	currency string,
) (*pbc.GetBudgetsResponse, error) {
	out := make([]*pbc.Budget, 0)
	if list == nil {
		return &pbc.GetBudgetsResponse{Budgets: out}, nil
	}
	now := time.Now().UTC()
	for _, link := range list.Values {
		if link.Id == "" || link.Name == "" {
			continue
		}
		budget, err := s.budgetFromLink(ctx, link, includeStatus, currency, now)
		if errors.Is(err, errNoBudgetAmount) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, budget)
	}
	return &pbc.GetBudgetsResponse{Budgets: out}, nil
}

func (s *FlexeraServer) budgetFromLink(
	ctx context.Context,
	link unified.BudgetBudgetLink,
	includeStatus bool,
	currency string,
	now time.Time,
) (*pbc.Budget, error) {
	report, err := s.api.BudgetReport(ctx, link.Id, budgetReportParams(link, now))
	if err != nil {
		return nil, mapFailure(err)
	}
	month := selectedBudgetMonth(link.YearMonths, now)
	limit, ok := monthBudgetAmount(report, month)
	if !ok {
		limit, ok, err = s.limitFromShow(ctx, link.Id, link.YearMonths, month)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errNoBudgetAmount
		}
	}
	budget := &pbc.Budget{
		Id:     link.Id,
		Name:   link.Name,
		Source: pluginName,
		Period: pbc.BudgetPeriod_BUDGET_PERIOD_MONTHLY,
		Amount: &pbc.BudgetAmount{Limit: limit, Currency: currency},
	}
	if includeStatus {
		budget.Status = budgetStatus(report, month, limit, currency)
	}
	return budget, nil
}

func (s *FlexeraServer) limitFromShow(
	ctx context.Context,
	id string,
	months []string,
	month time.Time,
) (float64, bool, error) {
	full, err := s.api.BudgetShow(ctx, id)
	if err != nil {
		return 0, false, mapFailure(err)
	}
	limit, ok := segmentBudgetAmount(full, months, month)
	return limit, ok, nil
}

func budgetReportParams(link unified.BudgetBudgetLink, now time.Time) *unified.BudgetBudgetReportParams {
	start := selectedBudgetMonth(link.YearMonths, now)
	summarized := false
	return &unified.BudgetBudgetReportParams{
		StartAt:    start.Format("2006-01"),
		EndAt:      start.AddDate(0, 1, 0).Format("2006-01"),
		Summarized: &summarized,
	}
}

func selectedBudgetMonth(months []string, now time.Time) time.Time {
	current := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	parsed := parseYearMonths(months)
	if len(parsed) == 0 {
		return current
	}
	for _, month := range parsed {
		if month.Equal(current) {
			return month
		}
	}
	first, last := parsed[0], parsed[0]
	for _, month := range parsed[1:] {
		if month.Before(first) {
			first = month
		}
		if month.After(last) {
			last = month
		}
	}
	if current.Before(first) {
		return first
	}
	if current.After(last) {
		return last
	}
	return current
}

func parseYearMonths(months []string) []time.Time {
	out := make([]time.Time, 0, len(months))
	for _, month := range months {
		parsed, err := time.Parse("2006-01", month)
		if err != nil {
			continue
		}
		out = append(out, parsed)
	}
	return out
}

func monthBudgetAmount(report *unified.BudgetBudgetReportRowList, month time.Time) (float64, bool) {
	if report == nil {
		return 0, false
	}
	var sum float64
	found := false
	for _, row := range report.Values {
		if row.Metrics.BudgetAmount == nil || !rowInMonth(row.Timestamp, month) {
			continue
		}
		sum += *row.Metrics.BudgetAmount
		found = true
	}
	return sum, found
}

func segmentBudgetAmount(full *unified.BudgetBudget, months []string, month time.Time) (float64, bool) {
	if full == nil {
		return 0, false
	}
	index := budgetMonthIndex(months, month)
	var sum float64
	found := false
	for _, segment := range full.Segments {
		amount, ok := segmentMonthAmount(segment.BudgetAmounts, index)
		if !ok {
			continue
		}
		sum += amount
		found = true
	}
	return sum, found
}

func budgetMonthIndex(months []string, month time.Time) int {
	for i, raw := range months {
		parsed, err := time.Parse("2006-01", raw)
		if err == nil && parsed.Equal(month) {
			return i
		}
	}
	return -1
}

func segmentMonthAmount(amounts []float64, index int) (float64, bool) {
	if len(amounts) == 0 {
		return 0, false
	}
	if index >= 0 && index < len(amounts) {
		return amounts[index], true
	}
	if index < 0 {
		return amounts[len(amounts)-1], true
	}
	return 0, false
}

func budgetStatus(
	report *unified.BudgetBudgetReportRowList,
	month time.Time,
	limit float64,
	currency string,
) *pbc.BudgetStatus {
	spend := spendAmount(report, month)
	pct := 0.0
	if limit > 0 {
		pct = spend / limit * fullPercent
	}
	return &pbc.BudgetStatus{
		CurrentSpend:   spend,
		PercentageUsed: pct,
		Currency:       currency,
		Health:         budgetHealth(pct),
	}
}

func spendAmount(report *unified.BudgetBudgetReportRowList, month time.Time) float64 {
	if report == nil {
		return 0
	}
	var sum float64
	for _, row := range report.Values {
		if row.Metrics.SpendAmount == nil || !rowInMonth(row.Timestamp, month) {
			continue
		}
		sum += *row.Metrics.SpendAmount
	}
	return sum
}

func rowInMonth(ts, month time.Time) bool {
	if ts.IsZero() {
		return true
	}
	ts = ts.UTC()
	return ts.Year() == month.Year() && ts.Month() == month.Month()
}

func budgetHealth(pct float64) pbc.BudgetHealthStatus {
	switch {
	case pct >= fullPercent:
		return pbc.BudgetHealthStatus_BUDGET_HEALTH_STATUS_EXCEEDED
	case pct >= budgetCriticalPercent:
		return pbc.BudgetHealthStatus_BUDGET_HEALTH_STATUS_CRITICAL
	case pct >= budgetWarnPercent:
		return pbc.BudgetHealthStatus_BUDGET_HEALTH_STATUS_WARNING
	default:
		return pbc.BudgetHealthStatus_BUDGET_HEALTH_STATUS_OK
	}
}
