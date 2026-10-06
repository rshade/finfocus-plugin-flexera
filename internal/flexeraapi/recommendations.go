package flexeraapi

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"

	flexera "github.com/flexera-public/unified-go-client"
)

// RecommendationsIndex calls the generated Optima recommendations index.
func (c *clientImpl) RecommendationsIndex(
	ctx context.Context,
	params *flexera.OptimaRecommendationsRecommendationsIndexParams,
) ([]flexera.OptimaRecommendationsRecommendationResultResponse, error) {
	resp, err := doWithRetry(
		ctx,
		c.retryPolicy(),
		func(ctx context.Context) (*flexera.OptimaRecommendationsRecommendationsIndexResponse, int, http.Header, error) {
			return c.recommendationsOnce(ctx, params)
		},
	)
	if err != nil {
		return nil, RedactError(fmt.Errorf("recommendations index call failed: %w", err))
	}
	if resp == nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		return nil, indexStatusErr(resp)
	}
	return append([]flexera.OptimaRecommendationsRecommendationResultResponse{}, (*resp.JSON200)...), nil
}

func (c *clientImpl) recommendationsOnce(
	ctx context.Context,
	params *flexera.OptimaRecommendationsRecommendationsIndexParams,
) (*flexera.OptimaRecommendationsRecommendationsIndexResponse, int, http.Header, error) {
	org, err := optimaOrgID(c.org)
	if err != nil {
		return nil, 0, nil, err
	}
	resp, err := c.client.OptimaRecommendationsRecommendationsIndexWithResponse(ctx, org, params)
	if err != nil || resp == nil {
		return nil, 0, nil, err
	}
	return resp, resp.StatusCode(), headerOf(resp.HTTPResponse), nil
}

// UpdateRecommendationStatus calls the generated Optima status update.
func (c *clientImpl) UpdateRecommendationStatus(
	ctx context.Context,
	body flexera.OptimaRecommendationsRecommendationsUpdateStatusRequestBody,
) error {
	resp, err := doWithRetry(
		ctx,
		c.retryPolicy(),
		func(ctx context.Context) (*flexera.OptimaRecommendationsRecommendationsUpdateStatusResponse, int, http.Header, error) {
			return c.updateRecommendationOnce(ctx, body)
		},
	)
	if err != nil {
		return RedactError(fmt.Errorf("recommendations updatestatus call failed: %w", err))
	}
	if resp == nil || !status2xx(resp.StatusCode()) {
		return updateStatusErr(resp)
	}
	return nil
}

func (c *clientImpl) updateRecommendationOnce(
	ctx context.Context,
	body flexera.OptimaRecommendationsRecommendationsUpdateStatusRequestBody,
) (*flexera.OptimaRecommendationsRecommendationsUpdateStatusResponse, int, http.Header, error) {
	org, err := optimaOrgID(c.org)
	if err != nil {
		return nil, 0, nil, err
	}
	resp, err := c.client.OptimaRecommendationsRecommendationsUpdateStatusWithResponse(ctx, org, body)
	if err != nil || resp == nil {
		return nil, 0, nil, err
	}
	return resp, resp.StatusCode(), headerOf(resp.HTTPResponse), nil
}

func optimaOrgID(org int64) (int, error) {
	if org <= 0 || org > int64(math.MaxInt) {
		return 0, fmt.Errorf("invalid org id: %d", org)
	}
	return int(org), nil
}

func headerOf(resp *http.Response) http.Header {
	if resp == nil {
		return nil
	}
	return resp.Header
}

func status2xx(code int) bool {
	return code >= http.StatusOK && code < http.StatusMultipleChoices
}

func indexStatusErr(resp *flexera.OptimaRecommendationsRecommendationsIndexResponse) error {
	if resp == nil {
		return errors.New("recommendations/index returned an empty response")
	}
	return statusErr("recommendations/index", resp.StatusCode(), resp.Body)
}

func updateStatusErr(resp *flexera.OptimaRecommendationsRecommendationsUpdateStatusResponse) error {
	if resp == nil {
		return errors.New("recommendations/updatestatus returned an empty response")
	}
	return statusErr("recommendations/updatestatus", resp.StatusCode(), resp.Body)
}
