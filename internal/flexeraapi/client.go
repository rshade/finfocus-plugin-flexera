package flexeraapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	flexera "github.com/flexera-public/unified-go-client"
)

// Client wraps the Flexera unified client for cost queries.
type Client interface {
	// CostsSelect queries the Bill Analysis API's costs/select endpoint.
	// Returns rows with timestamp, dimensions (key-value strings), and metrics as float64.
	// Metrics are JSON float64 values from Flexera; callers round each row to the
	// currency minor unit before summing.
	CostsSelect(ctx context.Context, req CostsSelectRequest) (*CostsSelectResponse, error)

	// CurrencyCode reads the org-wide settings/currency_code value.
	CurrencyCode(ctx context.Context) (string, error)

	// ForecastReport calls Bill Analysis forecasts/report.
	// The request and response fields follow the generated client models.
	ForecastReport(ctx context.Context, req ForecastRequest) (*ForecastResponse, error)
}

// ForecastRequest is the forecasts/report body from the generated client.
type ForecastRequest struct {
	BillingCenterIDs []string
	Dimensions       []string
	StartAt          string
	EndAt            string
	Granularity      string
	LookbackPeriod   int64
	Metric           string
	Filter           *FilterExpression
}

// ForecastResponse holds forecast segments from the generated client.
type ForecastResponse struct {
	Segments []ForecastSegment
}

// ForecastSegment is one dimension grouping from forecasts/report.
type ForecastSegment struct {
	ForecastAmounts []float64
	ActualAmounts   []float64
}

// CostsSelectRequest represents parameters for a costs/select query.
type CostsSelectRequest struct {
	// BillingCenterIDs is required; at least one must be provided.
	BillingCenterIDs []string `json:"billing_center_ids"`

	// Metrics to query; at least one required. Example: "cost_amortized_unblended_adj".
	Metrics []string `json:"metrics"`

	// Dimensions to return; at least one required. Example: "vendor", "service", "region".
	Dimensions []string `json:"dimensions"`

	// StartAt is required; format YYYY-MM or YYYY-MM-DD depending on granularity.
	StartAt string `json:"start_at"`

	// EndAt is required and exclusive; format YYYY-MM or YYYY-MM-DD.
	EndAt string `json:"end_at"`

	// Limit is required for /costs/select; max 100000.
	Limit int64 `json:"limit"`

	// Granularity is optional; "day" or "month" (default "month").
	Granularity *string `json:"granularity,omitempty"`

	// Filter is optional; expression tree with type, dimension, value, expressions.
	Filter *FilterExpression `json:"filter,omitempty"`
}

// FilterExpression represents a filter in the costs query.
type FilterExpression struct {
	Type        string              `json:"type"` // "equal", "substring", "and", "or", "not"
	Dimension   *string             `json:"dimension,omitempty"`
	Value       *string             `json:"value,omitempty"`
	Substring   *string             `json:"substring,omitempty"`
	Expression  *FilterExpression   `json:"expression,omitempty"`
	Expressions *[]FilterExpression `json:"expressions,omitempty"`
}

// CostsSelectResponse represents the response from costs/select.
type CostsSelectResponse struct {
	Rows []CostRow `json:"rows"`

	// Accepted is true when Flexera returned HTTP 202. Rows are still used when present.
	Accepted bool `json:"-"`

	// RowsTruncated is true when Flexera limited the result size.
	RowsTruncated bool `json:"-"`
}

// CostRow represents a single cost data row.
type CostRow struct {
	// Timestamp is the start of the day or month bucket (UTC).
	Timestamp time.Time `json:"timestamp"`

	// Dimensions are key-value strings (e.g., vendor, service, region, billing_center_id).
	Dimensions map[string]string `json:"dimensions"`

	// Metrics are numeric values from Flexera as JSON float64.
	// The plugin must convert these to decimal with proper rounding for currency operations.
	Metrics map[string]float64 `json:"metrics"`
}

// clientImpl wraps the Flexera unified client.
type clientImpl struct {
	org    int64
	client *flexera.ClientWithResponses
}

// New creates a new Client wrapper for Bill Analysis costs queries.
// zone: "us" (ZoneNAM), "eu" (ZoneEU), or "apac" (ZoneAPAC).
// orgID: numeric organization identifier.
// authCfg: OAuth config with RefreshToken or ClientID+ClientSecret.
// httpClient: optional HTTP client; if nil, a default is used.
func New(
	_ context.Context,
	zone Zone,
	orgID int64,
	authCfg flexera.OAuthConfig,
	httpClient *http.Client,
) (Client, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("invalid org id: %d", orgID)
	}

	// Map zone string to Flexera zone constant.
	flxZone, err := mapZone(zone)
	if err != nil {
		return nil, err
	}

	// Create auth helper.
	helper, err := flexera.NewAuthHelper(flexera.AuthHelperConfig{
		Zone:       flxZone,
		HTTPClient: httpClient,
	})
	if err != nil {
		return nil, fmt.Errorf("create auth helper: %w", err)
	}

	// Validate and create OAuth client.
	if err := helper.ValidateOAuth2Credentials(authCfg); err != nil {
		return nil, fmt.Errorf("validate oauth credentials: %w", err)
	}

	client, err := helper.NewOAuthClientWithResponses(authCfg)
	if err != nil {
		return nil, fmt.Errorf("create oauth client: %w", err)
	}

	return &clientImpl{org: orgID, client: client}, nil
}

// CostsSelect queries the Bill Analysis costs/select endpoint.
func (c *clientImpl) CostsSelect(ctx context.Context, req CostsSelectRequest) (*CostsSelectResponse, error) {
	if len(req.BillingCenterIDs) == 0 {
		return nil, errors.New("billing_center_ids is required")
	}
	if len(req.Metrics) == 0 {
		return nil, errors.New("metrics is required")
	}
	if len(req.Dimensions) == 0 {
		return nil, errors.New("dimensions is required")
	}
	if req.Limit <= 0 || req.Limit > 100000 {
		return nil, fmt.Errorf("limit must be between 1 and 100000, got %d", req.Limit)
	}

	// Build the request body for the unified client.
	billAnalysisReq := flexera.BillAnalysisSelectRequestBody{
		BillingCenterIds: req.BillingCenterIDs,
		Metrics:          req.Metrics,
		Dimensions:       req.Dimensions,
		StartAt:          req.StartAt,
		EndAt:            req.EndAt,
		Limit:            req.Limit,
	}

	// Set optional fields.
	if req.Granularity != nil {
		gran := flexera.BillAnalysisSelectRequestBodyGranularity(*req.Granularity)
		billAnalysisReq.Granularity = &gran
	}
	if req.Filter != nil {
		// Convert our filter expression to the unified client's format.
		billAnalysisReq.Filter = convertFilterExpression(req.Filter)
	}

	// Call the Flexera API.
	resp, err := c.client.BillAnalysisCostsSelectWithResponse(ctx, c.org, billAnalysisReq)
	if err != nil {
		return nil, fmt.Errorf("costs select call failed: %w", err)
	}

	payload, accepted, err := selectPayload(resp)
	if err != nil {
		return nil, err
	}

	result := &CostsSelectResponse{
		Rows:     make([]CostRow, len(payload.Rows)),
		Accepted: accepted,
	}
	if payload.RowsTruncated != nil {
		result.RowsTruncated = *payload.RowsTruncated
	}
	for i, row := range payload.Rows {
		result.Rows[i] = CostRow{
			Timestamp:  row.Timestamp,
			Dimensions: row.Dimensions,
			Metrics:    row.Metrics,
		}
	}

	return result, nil
}

// CurrencyCode reads settings/currency_code for the org.
func (c *clientImpl) CurrencyCode(ctx context.Context) (string, error) {
	resp, err := c.client.BillAnalysisCurrencySettingShowWithResponse(ctx, c.org)
	if err != nil {
		return "", fmt.Errorf("currency_code call failed: %w", err)
	}
	if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		return "", fmt.Errorf("currency_code returned %d", resp.StatusCode())
	}
	code := strings.TrimSpace(resp.JSON200.Value)
	if code == "" {
		return "", errors.New("currency_code is empty")
	}
	return code, nil
}

// ForecastReport calls the generated BillAnalysisForecastsReport method.
func (c *clientImpl) ForecastReport(ctx context.Context, req ForecastRequest) (*ForecastResponse, error) {
	if len(req.BillingCenterIDs) == 0 {
		return nil, errors.New("billing_center_ids is required")
	}
	if len(req.Dimensions) == 0 {
		return nil, errors.New("dimensions is required")
	}
	if req.LookbackPeriod <= 0 {
		return nil, errors.New("lookback period is required")
	}
	body := flexera.BillAnalysisReportRequestBody3{
		BillingCenterIds: req.BillingCenterIDs,
		Dimensions:       req.Dimensions,
		StartAt:          req.StartAt,
		EndAt:            req.EndAt,
		LookbackPeriod:   req.LookbackPeriod,
		Granularity:      flexera.BillAnalysisReportRequestBody3Granularity(req.Granularity),
		Metric:           flexera.BillAnalysisReportRequestBody3Metric(req.Metric),
		Filter:           convertFilterExpression(req.Filter),
	}
	resp, err := c.client.BillAnalysisForecastsReportWithResponse(ctx, c.org, body)
	if err != nil {
		return nil, fmt.Errorf("forecasts/report call failed: %w", err)
	}
	if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil || resp.JSON200.Segments == nil {
		return nil, fmt.Errorf("forecasts/report returned %d", resp.StatusCode())
	}
	out := &ForecastResponse{Segments: make([]ForecastSegment, len(*resp.JSON200.Segments))}
	for i, segment := range *resp.JSON200.Segments {
		if segment.ForecastAmounts != nil {
			out.Segments[i].ForecastAmounts = append([]float64{}, (*segment.ForecastAmounts)...)
		}
		if segment.ActualAmounts != nil {
			out.Segments[i].ActualAmounts = append([]float64{}, (*segment.ActualAmounts)...)
		}
	}
	return out, nil
}

func selectPayload(
	resp *flexera.BillAnalysisCostsSelectResponse,
) (*flexera.BillAnalysisAnalyticsQueryResult, bool, error) {
	if resp == nil {
		return nil, false, errors.New("empty response from costs/select")
	}
	switch resp.StatusCode() {
	case http.StatusOK:
		if resp.JSON200 == nil {
			return nil, false, errors.New("empty response from costs/select")
		}
		return resp.JSON200, false, nil
	case http.StatusAccepted:
		payload := resp.JSON202
		if payload == nil {
			payload = resp.JSON200
		}
		if payload == nil {
			return nil, true, errors.New("costs select returned 202 without rows")
		}
		return payload, true, nil
	default:
		return nil, false, fmt.Errorf("costs select returned %d: %s", resp.StatusCode(), resp.Body)
	}
}

// convertFilterExpression converts our FilterExpression to the unified client's format.
func convertFilterExpression(expr *FilterExpression) *flexera.BillAnalysisFilterV1 {
	if expr == nil {
		return nil
	}

	filterType := flexera.BillAnalysisFilterV1Type(expr.Type)
	out := &flexera.BillAnalysisFilterV1{
		Type: filterType,
	}

	if expr.Dimension != nil {
		out.Dimension = expr.Dimension
	}
	if expr.Value != nil {
		out.Value = expr.Value
	}
	if expr.Substring != nil {
		out.Substring = expr.Substring
	}
	if expr.Expression != nil {
		out.Expression = convertFilterExpression(expr.Expression)
	}
	if expr.Expressions != nil {
		exprs := make([]flexera.BillAnalysisFilterV1, len(*expr.Expressions))
		for i := range *expr.Expressions {
			if converted := convertFilterExpression(&(*expr.Expressions)[i]); converted != nil {
				exprs[i] = *converted
			}
		}
		out.Expressions = &exprs
	}

	return out
}

// Zone is a string alias for region selection.
type Zone string

const (
	ZoneUS   Zone = "us"
	ZoneEU   Zone = "eu"
	ZoneAPAC Zone = "apac"
)

// ZoneForRegion maps a plugin region (nam, eu, apac) to a login zone.
func ZoneForRegion(region string) (Zone, error) {
	switch strings.ToLower(strings.TrimSpace(region)) {
	case "nam", "north-america", "us", "":
		return ZoneUS, nil
	case "eu", "europe":
		return ZoneEU, nil
	case "apac", "asia-pacific", "au":
		return ZoneAPAC, nil
	default:
		return "", fmt.Errorf("invalid flexera region %q", region)
	}
}

// mapZone maps our Zone enum to the Flexera zone constant.
func mapZone(z Zone) (flexera.Zone, error) {
	switch z {
	case ZoneUS:
		return flexera.ZoneNAM, nil
	case ZoneEU:
		return flexera.ZoneEU, nil
	case ZoneAPAC:
		return flexera.ZoneAPAC, nil
	default:
		return "", fmt.Errorf("invalid zone: %q", z)
	}
}
