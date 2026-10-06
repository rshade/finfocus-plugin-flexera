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

	// RecommendationsIndex calls Optima recommendations index.
	// The result items are the generated recommendation models.
	RecommendationsIndex(
		ctx context.Context,
		params *flexera.OptimaRecommendationsRecommendationsIndexParams,
	) ([]flexera.OptimaRecommendationsRecommendationResultResponse, error)

	// UpdateRecommendationStatus calls Optima recommendations updatestatus.
	// A 2xx response is success. The generated response has no success body.
	UpdateRecommendationStatus(
		ctx context.Context,
		body flexera.OptimaRecommendationsRecommendationsUpdateStatusRequestBody,
	) error

	// BudgetIndex calls the read-only budget index. Links carry id and name.
	BudgetIndex(ctx context.Context) (*flexera.BudgetBudgetList, error)

	// BudgetShow calls the read-only budget show. Segments carry budgetAmounts.
	BudgetShow(ctx context.Context, id string) (*flexera.BudgetBudget, error)

	// BudgetReport calls the read-only budget report. Rows carry budget and spend metrics.
	BudgetReport(
		ctx context.Context,
		id string,
		params *flexera.BudgetBudgetReportParams,
	) (*flexera.BudgetBudgetReportRowList, error)
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
	retry  retryPolicy
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
	if credErr := helper.ValidateOAuth2Credentials(authCfg); credErr != nil {
		return nil, RedactError(fmt.Errorf("validate oauth credentials: %w", credErr))
	}

	client, err := helper.NewOAuthClientWithResponses(authCfg)
	if err != nil {
		return nil, RedactError(fmt.Errorf("create oauth client: %w", err))
	}

	return &clientImpl{org: orgID, client: client}, nil
}

// CostsSelect queries the Bill Analysis costs/select endpoint.
func (c *clientImpl) CostsSelect(ctx context.Context, req CostsSelectRequest) (*CostsSelectResponse, error) {
	if err := validateSelect(req); err != nil {
		return nil, err
	}
	body := selectBody(req)
	resp, err := doWithRetry(
		ctx,
		c.retryPolicy(),
		func(ctx context.Context) (*flexera.BillAnalysisCostsSelectResponse, int, http.Header, error) {
			return c.selectOnce(ctx, body)
		},
	)
	if err != nil {
		return nil, RedactError(fmt.Errorf("costs select call failed: %w", err))
	}
	payload, accepted, err := selectPayload(resp)
	if err != nil {
		return nil, err
	}
	return rowsFromPayload(payload, accepted), nil
}

func validateSelect(req CostsSelectRequest) error {
	if len(req.BillingCenterIDs) == 0 {
		return errors.New("billing_center_ids is required")
	}
	if len(req.Metrics) == 0 {
		return errors.New("metrics is required")
	}
	if len(req.Dimensions) == 0 {
		return errors.New("dimensions is required")
	}
	if req.Limit <= 0 || req.Limit > 100000 {
		return fmt.Errorf("limit must be between 1 and 100000, got %d", req.Limit)
	}
	return nil
}

func selectBody(req CostsSelectRequest) flexera.BillAnalysisSelectRequestBody {
	body := flexera.BillAnalysisSelectRequestBody{
		BillingCenterIds: req.BillingCenterIDs,
		Metrics:          req.Metrics,
		Dimensions:       req.Dimensions,
		StartAt:          req.StartAt,
		EndAt:            req.EndAt,
		Limit:            req.Limit,
	}
	if req.Granularity != nil {
		gran := flexera.BillAnalysisSelectRequestBodyGranularity(*req.Granularity)
		body.Granularity = &gran
	}
	if req.Filter != nil {
		body.Filter = convertFilterExpression(req.Filter)
	}
	return body
}

func (c *clientImpl) selectOnce(
	ctx context.Context,
	body flexera.BillAnalysisSelectRequestBody,
) (*flexera.BillAnalysisCostsSelectResponse, int, http.Header, error) {
	resp, err := c.client.BillAnalysisCostsSelectWithResponse(ctx, c.org, body)
	if err != nil || resp == nil {
		return nil, 0, nil, err
	}
	return resp, resp.StatusCode(), headerFrom(resp), nil
}

func rowsFromPayload(payload *flexera.BillAnalysisAnalyticsQueryResult, accepted bool) *CostsSelectResponse {
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
	return result
}

// CurrencyCode reads settings/currency_code for the org.
func (c *clientImpl) CurrencyCode(ctx context.Context) (string, error) {
	resp, err := doWithRetry(
		ctx,
		c.retryPolicy(),
		func(ctx context.Context) (*flexera.BillAnalysisCurrencySettingShowResponse, int, http.Header, error) {
			return c.currencyOnce(ctx)
		},
	)
	if err != nil {
		return "", RedactError(fmt.Errorf("currency_code call failed: %w", err))
	}
	if resp == nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		status := 0
		var body []byte
		if resp != nil {
			status = resp.StatusCode()
			body = resp.Body
		}
		return "", statusErr("currency_code", status, body)
	}
	code := strings.TrimSpace(resp.JSON200.Value)
	if code == "" {
		return "", errors.New("currency_code is empty")
	}
	return code, nil
}

func (c *clientImpl) currencyOnce(
	ctx context.Context,
) (*flexera.BillAnalysisCurrencySettingShowResponse, int, http.Header, error) {
	resp, err := c.client.BillAnalysisCurrencySettingShowWithResponse(ctx, c.org)
	if err != nil || resp == nil {
		return nil, 0, nil, err
	}
	return resp, resp.StatusCode(), currencyHeader(resp), nil
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
	resp, err := doWithRetry(
		ctx,
		c.retryPolicy(),
		func(ctx context.Context) (*flexera.BillAnalysisForecastsReportResponse, int, http.Header, error) {
			return c.forecastOnce(ctx, body)
		},
	)
	if err != nil {
		return nil, RedactError(fmt.Errorf("forecasts/report call failed: %w", err))
	}
	if resp == nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil || resp.JSON200.Segments == nil {
		status := 0
		var raw []byte
		if resp != nil {
			status = resp.StatusCode()
			raw = resp.Body
		}
		return nil, statusErr("forecasts/report", status, raw)
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

func (c *clientImpl) forecastOnce(
	ctx context.Context,
	body flexera.BillAnalysisReportRequestBody3,
) (*flexera.BillAnalysisForecastsReportResponse, int, http.Header, error) {
	resp, err := c.client.BillAnalysisForecastsReportWithResponse(ctx, c.org, body)
	if err != nil || resp == nil {
		return nil, 0, nil, err
	}
	return resp, resp.StatusCode(), forecastHeader(resp), nil
}

func headerFrom(resp *flexera.BillAnalysisCostsSelectResponse) http.Header {
	if resp == nil || resp.HTTPResponse == nil {
		return nil
	}
	return resp.HTTPResponse.Header
}

func currencyHeader(resp *flexera.BillAnalysisCurrencySettingShowResponse) http.Header {
	if resp == nil || resp.HTTPResponse == nil {
		return nil
	}
	return resp.HTTPResponse.Header
}

func forecastHeader(resp *flexera.BillAnalysisForecastsReportResponse) http.Header {
	if resp == nil || resp.HTTPResponse == nil {
		return nil
	}
	return resp.HTTPResponse.Header
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
		return nil, false, statusErr("costs/select", resp.StatusCode(), resp.Body)
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
