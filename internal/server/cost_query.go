package server

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/rshade/finfocus-plugin-flexera/internal/flexeraapi"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type dayWindow struct {
	Start time.Time
	End   time.Time
}

func (s *FlexeraServer) forecastMonthly(
	ctx context.Context,
	filter *flexeraapi.FilterExpression,
) (float64, bool, error) {
	if s == nil || s.api == nil || len(s.billingCenterIDs) == 0 {
		return 0, false, nil
	}
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	resp, err := s.api.ForecastReport(ctx, flexeraapi.ForecastRequest{
		BillingCenterIDs: s.billingCenterIDs,
		Dimensions:       []string{dimVendor, dimService},
		StartAt:          start.Format("2006-01"),
		EndAt:            end.Format("2006-01"),
		Granularity:      "month",
		LookbackPeriod:   forecastLookbackMonths,
		Metric:           s.costMetric(),
		Filter:           filter,
	})
	if err != nil {
		return 0, false, err
	}
	return sumForecast(resp), forecastHasAmounts(resp), nil
}

func forecastHasAmounts(resp *flexeraapi.ForecastResponse) bool {
	if resp == nil {
		return false
	}
	for _, segment := range resp.Segments {
		if len(segment.ForecastAmounts) > 0 {
			return true
		}
	}
	return false
}

func sumForecast(resp *flexeraapi.ForecastResponse) float64 {
	if resp == nil {
		return 0
	}
	var total float64
	for _, segment := range resp.Segments {
		for _, amount := range segment.ForecastAmounts {
			total += amount
		}
	}
	return total
}

func (s *FlexeraServer) costMetric() string {
	if s != nil && s.metric != "" {
		return s.metric
	}
	return defaultCostMetric
}

func (s *FlexeraServer) currencyCode(ctx context.Context) (string, error) {
	if s == nil || s.api == nil {
		return "", status.Error(codes.FailedPrecondition, "flexera api client is not configured")
	}
	now := time.Now()
	s.currency.mu.Lock()
	if s.currency.code != "" && now.Before(s.currency.expires) {
		code := s.currency.code
		s.currency.mu.Unlock()
		return code, nil
	}
	s.currency.mu.Unlock()

	code, err := s.api.CurrencyCode(ctx)
	if err != nil {
		return "", mapFailure(err)
	}
	s.currency.mu.Lock()
	s.currency.code = code
	s.currency.expires = time.Now().Add(currencyCacheTTL)
	s.currency.mu.Unlock()
	return code, nil
}

func chunkDayWindows(start, end time.Time) ([]dayWindow, error) {
	startDay := utcDate(start)
	endDay := exclusiveEndDate(end)
	if !startDay.Before(endDay) {
		return nil, errors.New("start must be before end")
	}
	var windows []dayWindow
	for cursor := startDay; cursor.Before(endDay); {
		next := cursor.AddDate(0, 0, maxSelectDays)
		if next.After(endDay) {
			next = endDay
		}
		windows = append(windows, dayWindow{Start: cursor, End: next})
		cursor = next
	}
	return windows, nil
}

func utcDate(ts time.Time) time.Time {
	utc := ts.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func exclusiveEndDate(ts time.Time) time.Time {
	utc := ts.UTC()
	day := utcDate(utc)
	if utc.Equal(day) {
		return day
	}
	return day.AddDate(0, 0, 1)
}

func costFilter(resourceID string) (*flexeraapi.FilterExpression, error) {
	parts := strings.Split(resourceID, "/")
	switch parts[0] {
	case dimService:
		if len(parts) < triple {
			return nil, errors.New("service id must be service/<vendor>/<name>")
		}
		return andFilter(equalFilter(dimVendor, parts[1]), equalFilter(dimService, parts[2])), nil
	case "vendor_account":
		if len(parts) < pair || parts[1] == "" || parts[1] == "*" {
			return nil, errors.New("vendor account id is required")
		}
		return equalFilter("vendor_account", parts[1]), nil
	case dimRegion:
		if len(parts) < triple {
			return nil, errors.New("region id must be region/<vendor>/<region>")
		}
		return andFilter(equalFilter(dimVendor, parts[1]), equalFilter(dimRegion, parts[2])), nil
	case "resource_group":
		if len(parts) < pair || parts[1] == "" {
			return nil, errors.New("resource group id is required")
		}
		return equalFilter("resource_group", parts[1]), nil
	case "billing_center":
		return nil, errors.New("billing center is set with billing_center_ids, not a filter")
	default:
		return equalFilter(dimResourceID, resourceID), nil
	}
}

func equalFilter(dimension, value string) *flexeraapi.FilterExpression {
	return &flexeraapi.FilterExpression{
		Type:      "equal",
		Dimension: &dimension,
		Value:     &value,
	}
}

func andFilter(parts ...*flexeraapi.FilterExpression) *flexeraapi.FilterExpression {
	exprs := make([]flexeraapi.FilterExpression, 0, len(parts))
	for _, part := range parts {
		if part != nil {
			exprs = append(exprs, *part)
		}
	}
	return &flexeraapi.FilterExpression{Type: "and", Expressions: &exprs}
}

func roundCurrency(amount float64, currency string) float64 {
	digits := minorDigits(currency)
	scale := math.Pow10(digits)
	return math.Round(amount*scale) / scale
}

func minorDigits(currency string) int {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "JPY", "KRW", "VND":
		return minorDigitsNone
	case "BHD", "KWD", "OMR":
		return minorDigitsThree
	default:
		return minorDigitsDefault
	}
}

func projectMonthly(results []*pbc.ActualCostResult, currency string) (float64, float64) {
	digits := minorDigits(currency)
	scale := math.Pow10(digits)
	var totalMinor int64
	for _, point := range results {
		totalMinor += int64(math.Round(point.GetCost() * scale))
	}
	count := int64(len(results))
	dailyMinor := totalMinor / count
	monthlyMinor := dailyMinor * daysPerMonth
	if len(results) >= trendMinPoints {
		mid := len(results) / pair
		first := averageMinor(results[:mid], scale)
		second := averageMinor(results[mid:], scale)
		if first > 0 {
			monthlyMinor = int64(math.Round(float64(monthlyMinor) * float64(second) / float64(first)))
		}
	}
	return float64(dailyMinor) / scale, float64(monthlyMinor) / scale
}

func averageMinor(results []*pbc.ActualCostResult, scale float64) int64 {
	if len(results) == 0 {
		return 0
	}
	var total int64
	for _, point := range results {
		total += int64(math.Round(point.GetCost() * scale))
	}
	return total / int64(len(results))
}

func sortCostResults(results []*pbc.ActualCostResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].GetTimestamp().AsTime().Before(results[j].GetTimestamp().AsTime())
	})
}
