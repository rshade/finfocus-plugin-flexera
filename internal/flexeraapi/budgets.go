package flexeraapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	flexera "github.com/flexera-public/unified-go-client"
)

// BudgetIndex calls the generated budget index.
func (c *clientImpl) BudgetIndex(ctx context.Context) (*flexera.BudgetBudgetList, error) {
	resp, err := doWithRetry(
		ctx,
		c.retryPolicy(),
		func(ctx context.Context) (*flexera.BudgetBudgetIndexResponse, int, http.Header, error) {
			return c.budgetIndexOnce(ctx)
		},
	)
	if err != nil {
		return nil, RedactError(fmt.Errorf("budgets index call failed: %w", err))
	}
	if resp == nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		return nil, budgetIndexErr(resp)
	}
	return resp.JSON200, nil
}

func (c *clientImpl) budgetIndexOnce(
	ctx context.Context,
) (*flexera.BudgetBudgetIndexResponse, int, http.Header, error) {
	org, err := optimaOrgID(c.org)
	if err != nil {
		return nil, 0, nil, err
	}
	resp, err := c.client.BudgetBudgetIndexWithResponse(ctx, org)
	if err != nil || resp == nil {
		return nil, 0, nil, err
	}
	return resp, resp.StatusCode(), headerOf(resp.HTTPResponse), nil
}

// BudgetShow calls the generated budget show.
func (c *clientImpl) BudgetShow(ctx context.Context, id string) (*flexera.BudgetBudget, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("budget id is required")
	}
	resp, err := doWithRetry(
		ctx,
		c.retryPolicy(),
		func(ctx context.Context) (*flexera.BudgetBudgetShowResponse, int, http.Header, error) {
			return c.budgetShowOnce(ctx, id)
		},
	)
	if err != nil {
		return nil, RedactError(fmt.Errorf("budgets show call failed: %w", err))
	}
	if resp == nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		return nil, budgetShowErr(resp)
	}
	return resp.JSON200, nil
}

func (c *clientImpl) budgetShowOnce(
	ctx context.Context,
	id string,
) (*flexera.BudgetBudgetShowResponse, int, http.Header, error) {
	org, err := optimaOrgID(c.org)
	if err != nil {
		return nil, 0, nil, err
	}
	resp, err := c.client.BudgetBudgetShowWithResponse(ctx, org, id)
	if err != nil || resp == nil {
		return nil, 0, nil, err
	}
	return resp, resp.StatusCode(), headerOf(resp.HTTPResponse), nil
}

// BudgetReport calls the generated budget report.
func (c *clientImpl) BudgetReport(
	ctx context.Context,
	id string,
	params *flexera.BudgetBudgetReportParams,
) (*flexera.BudgetBudgetReportRowList, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("budget id is required")
	}
	if params == nil || params.StartAt == "" || params.EndAt == "" {
		return nil, errors.New("budget report startAt and endAt are required")
	}
	resp, err := doWithRetry(
		ctx,
		c.retryPolicy(),
		func(ctx context.Context) (*flexera.BudgetBudgetReportResponse, int, http.Header, error) {
			return c.budgetReportOnce(ctx, id, params)
		},
	)
	if err != nil {
		return nil, RedactError(fmt.Errorf("budgets report call failed: %w", err))
	}
	if resp == nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		return nil, budgetReportErr(resp)
	}
	return resp.JSON200, nil
}

func (c *clientImpl) budgetReportOnce(
	ctx context.Context,
	id string,
	params *flexera.BudgetBudgetReportParams,
) (*flexera.BudgetBudgetReportResponse, int, http.Header, error) {
	org, err := optimaOrgID(c.org)
	if err != nil {
		return nil, 0, nil, err
	}
	resp, err := c.client.BudgetBudgetReportWithResponse(ctx, org, id, params)
	if err != nil || resp == nil {
		return nil, 0, nil, err
	}
	return resp, resp.StatusCode(), headerOf(resp.HTTPResponse), nil
}

func budgetIndexErr(resp *flexera.BudgetBudgetIndexResponse) error {
	if resp == nil {
		return errors.New("budgets index returned an empty response")
	}
	return statusErr("budgets/index", resp.StatusCode(), resp.Body)
}

func budgetShowErr(resp *flexera.BudgetBudgetShowResponse) error {
	if resp == nil {
		return errors.New("budgets show returned an empty response")
	}
	return statusErr("budgets/show", resp.StatusCode(), resp.Body)
}

func budgetReportErr(resp *flexera.BudgetBudgetReportResponse) error {
	if resp == nil {
		return errors.New("budgets report returned an empty response")
	}
	return statusErr("budgets/report", resp.StatusCode(), resp.Body)
}
