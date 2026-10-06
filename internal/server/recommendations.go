package server

import (
	"context"
	"strconv"
	"strings"
	"time"

	unified "github.com/flexera-public/unified-go-client"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	maxTargetResources    = 100
	defaultPageSize       = 50
	maxPageSize           = 1000
	maxDismissalReasonLen = 500
	defaultProjection     = "monthly"
	missingFlexeraClient  = "flexera api client is not configured"
)

// GetRecommendations maps Optima recommendation index rows onto finfocus recommendations.
func (s *FlexeraServer) GetRecommendations(
	ctx context.Context,
	req *pbc.GetRecommendationsRequest,
) (*pbc.GetRecommendationsResponse, error) {
	if err := validateRecommendationsRequest(req); err != nil {
		return nil, err
	}
	ctx, cancel, err := s.beginRPC(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	if s == nil || s.api == nil {
		return nil, status.Error(codes.FailedPrecondition, missingFlexeraClient)
	}
	rows, err := s.api.RecommendationsIndex(ctx, s.recommendationParams(req))
	if err != nil {
		return nil, mapFailure(err)
	}
	currency, err := s.currencyCode(ctx)
	if err != nil {
		return nil, err
	}
	page, next := s.visibleRecommendations(req, rows, currency)
	return recommendationsFrom(page, next), nil
}

// DismissRecommendation records a dismissal upstream and in this process.
func (s *FlexeraServer) DismissRecommendation(
	ctx context.Context,
	req *pbc.DismissRecommendationRequest,
) (*pbc.DismissRecommendationResponse, error) {
	if err := validateDismissal(req); err != nil {
		return nil, err
	}
	ctx, cancel, err := s.beginRPC(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	if s == nil || s.api == nil {
		return nil, status.Error(codes.FailedPrecondition, missingFlexeraClient)
	}
	updateErr := s.api.UpdateRecommendationStatus(ctx, dismissalBody(req))
	if updateErr != nil {
		return nil, mapFailure(updateErr)
	}
	s.rememberDismissal(req.GetRecommendationId(), req.GetExpiresAt())
	return &pbc.DismissRecommendationResponse{
		Success:          true,
		Message:          "dismissed",
		DismissedAt:      timestamppb.New(time.Now().UTC()),
		ExpiresAt:        req.GetExpiresAt(),
		RecommendationId: req.GetRecommendationId(),
	}, nil
}

func validateRecommendationsRequest(req *pbc.GetRecommendationsRequest) error {
	if req == nil {
		return status.Error(codes.InvalidArgument, "request is required")
	}
	if len(req.GetTargetResources()) > maxTargetResources {
		return status.Error(codes.InvalidArgument, "target_resources exceeds 100")
	}
	if _, err := pageOffset(req.GetPageToken()); err != nil {
		return err
	}
	return nil
}

func validateDismissal(req *pbc.DismissRecommendationRequest) error {
	if req == nil || strings.TrimSpace(req.GetRecommendationId()) == "" {
		return status.Error(codes.InvalidArgument, "recommendation_id is required")
	}
	if len(req.GetCustomReason()) > maxDismissalReasonLen {
		return status.Error(codes.InvalidArgument, "custom_reason exceeds 500 characters")
	}
	return nil
}

func (s *FlexeraServer) recommendationParams(
	req *pbc.GetRecommendationsRequest,
) *unified.OptimaRecommendationsRecommendationsIndexParams {
	params := &unified.OptimaRecommendationsRecommendationsIndexParams{}
	if s != nil && len(s.billingCenterIDs) > 0 {
		ids := append([]string{}, s.billingCenterIDs...)
		params.BillingCenterIDs = &ids
	}
	statuses := []unified.OptimaRecommendationsRecommendationsIndexParamsStatuses{unified.Active}
	if req.GetIncludeDismissed() {
		statuses = []unified.OptimaRecommendationsRecommendationsIndexParamsStatuses{
			unified.Active,
			unified.Realized,
			unified.Rejected,
			unified.Snoozed,
		}
	}
	params.Statuses = &statuses
	return params
}

func (s *FlexeraServer) visibleRecommendations(
	req *pbc.GetRecommendationsRequest,
	rows []unified.OptimaRecommendationsRecommendationResultResponse,
	currency string,
) ([]*pbc.Recommendation, string) {
	excluded := idSet(req.GetExcludedRecommendationIds())
	include := req.GetIncludeDismissed()
	period := projectionPeriod(req)
	limit := pageLimit(req)
	offset, _ := pageOffset(req.GetPageToken())
	now := time.Now()
	seen := 0
	out := make([]*pbc.Recommendation, 0, limit)
	for _, row := range rows {
		if s.hideRecommendation(row, excluded, include, now) {
			continue
		}
		if seen < offset {
			seen++
			continue
		}
		if len(out) == limit {
			return out, strconv.Itoa(offset + len(out))
		}
		out = append(out, mapRecommendation(row, currency, period))
	}
	return out, ""
}

func recommendationsFrom(recs []*pbc.Recommendation, next string) *pbc.GetRecommendationsResponse {
	currency := ""
	period := defaultProjection
	if len(recs) > 0 && recs[0].GetImpact() != nil {
		currency = recs[0].GetImpact().GetCurrency()
		period = recs[0].GetImpact().GetProjectionPeriod()
	}
	return &pbc.GetRecommendationsResponse{
		Recommendations: recs,
		NextPageToken:   next,
		Summary:         recommendationSummary(recs, currency, period),
	}
}

func (s *FlexeraServer) hideRecommendation(
	row unified.OptimaRecommendationsRecommendationResultResponse,
	excluded map[string]struct{},
	includeDismissed bool,
	now time.Time,
) bool {
	if _, ok := excluded[row.Id]; ok {
		return true
	}
	if includeDismissed {
		return false
	}
	return dismissedStatus(row.Status) || s.dismissalActive(row.Id, now)
}

func mapRecommendation(
	src unified.OptimaRecommendationsRecommendationResultResponse,
	currency, period string,
) *pbc.Recommendation {
	rec := &pbc.Recommendation{
		Id:          src.Id,
		Category:    pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_COST,
		ActionType:  actionForRecommendation(src.Type),
		Description: src.Recommendation,
		Source:      pluginName,
		Impact: &pbc.RecommendationImpact{
			EstimatedSavings: src.Savings,
			Currency:         currency,
			ProjectionPeriod: period,
		},
	}
	if src.ResourceID != "" {
		rec.Resource = &pbc.ResourceRecommendationInfo{
			Id:           src.ResourceID,
			Provider:     strings.ToLower(src.Vendor),
			ResourceType: src.ResourceType,
			Region:       src.Region,
		}
	}
	if !src.CreatedAt.IsZero() {
		rec.CreatedAt = timestamppb.New(src.CreatedAt)
	}
	return rec
}

func actionForRecommendation(
	kind unified.OptimaRecommendationsRecommendationResultResponseType,
) pbc.RecommendationActionType {
	switch kind {
	case unified.RateReduction:
		return pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_PURCHASE_COMMITMENT
	case unified.UsageReduction:
		return pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_RIGHTSIZE
	default:
		return pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_OTHER
	}
}

func recommendationSummary(recs []*pbc.Recommendation, currency, period string) *pbc.RecommendationSummary {
	var total float64
	var count int32
	for _, rec := range recs {
		total += rec.GetImpact().GetEstimatedSavings()
		count++
	}
	return &pbc.RecommendationSummary{
		TotalRecommendations:  count,
		TotalEstimatedSavings: total,
		Currency:              currency,
		ProjectionPeriod:      period,
	}
}

func dismissalBody(
	req *pbc.DismissRecommendationRequest,
) unified.OptimaRecommendationsRecommendationsUpdateStatusRequestBody {
	body := unified.OptimaRecommendationsRecommendationsUpdateStatusRequestBody{
		Id:           req.GetRecommendationId(),
		Status:       unified.OptimaRecommendationsRecommendationsUpdateStatusRequestBodyStatusRejected,
		StatusReason: statusReason(req),
	}
	if req.GetExpiresAt() == nil {
		return body
	}
	date := req.GetExpiresAt().AsTime().UTC().Format(time.DateOnly)
	body.Status = unified.OptimaRecommendationsRecommendationsUpdateStatusRequestBodyStatusSnoozed
	body.SnoozedTargetDate = &date
	return body
}

func statusReason(req *pbc.DismissRecommendationRequest) *string {
	if text := strings.TrimSpace(req.GetCustomReason()); text != "" {
		return &text
	}
	if req.GetReason() == pbc.DismissalReason_DISMISSAL_REASON_UNSPECIFIED {
		return nil
	}
	text := req.GetReason().String()
	return &text
}

func (s *FlexeraServer) rememberDismissal(id string, expires *timestamppb.Timestamp) {
	if s == nil {
		return
	}
	var until time.Time
	if expires != nil {
		until = expires.AsTime()
	}
	s.dismissMu.Lock()
	defer s.dismissMu.Unlock()
	if s.dismissed == nil {
		s.dismissed = map[string]time.Time{}
	}
	s.dismissed[id] = until
}

func (s *FlexeraServer) dismissalActive(id string, now time.Time) bool {
	if s == nil {
		return false
	}
	s.dismissMu.Lock()
	defer s.dismissMu.Unlock()
	until, ok := s.dismissed[id]
	if !ok {
		return false
	}
	if !until.IsZero() && !now.Before(until) {
		delete(s.dismissed, id)
		return false
	}
	return true
}

func dismissedStatus(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(unified.Snoozed), string(unified.Rejected):
		return true
	default:
		return false
	}
}

func projectionPeriod(req *pbc.GetRecommendationsRequest) string {
	if period := strings.TrimSpace(req.GetProjectionPeriod()); period != "" {
		return period
	}
	return defaultProjection
}

func pageOffset(token string) (int, error) {
	if strings.TrimSpace(token) == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(strings.TrimSpace(token))
	if err != nil || offset < 0 {
		return 0, status.Error(codes.InvalidArgument, "page_token is invalid")
	}
	return offset, nil
}

func pageLimit(req *pbc.GetRecommendationsRequest) int {
	n := int(req.GetPageSize())
	if n <= 0 {
		return defaultPageSize
	}
	if n > maxPageSize {
		return maxPageSize
	}
	return n
}

func idSet(ids []string) map[string]struct{} {
	if len(ids) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id != "" {
			out[id] = struct{}{}
		}
	}
	return out
}
